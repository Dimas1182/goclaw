#!/usr/bin/env python3
"""
Gemini Image Generation Helper Script

This script generates images using Google's Gemini 3.0 Preview Image model
with automatic API key detection and file saving to ./docs/assets/

Usage:
    python generate.py "Your prompt here" [options]

Examples:
    python generate.py "A serene mountain landscape"
    python generate.py "Futuristic city" --output city.png
    python generate.py "Modern design" --include-text
"""

import os
import sys
import argparse
from pathlib import Path
from datetime import datetime
from typing import Optional, List

try:
    from google import genai
    from google.genai.types import GenerateContentConfig, Modality
except ImportError:
    print("Error: google-genai package not installed")
    print("Install with: pip install google-genai")
    sys.exit(1)


def find_api_key() -> Optional[str]:
    """
    Find API key in this order:
    1. GOOGLE_API_KEY (process environment)
    2. GEMINI_API_KEY (process environment)
    3. Skill directory .env file
    4. Project directory .env file

    Returns:
        API key string or None if not found
    """
    # 1. Check GOOGLE_API_KEY first (preferred by google-genai)
    api_key = os.getenv('GOOGLE_API_KEY')
    if api_key:
        print("✓ API key found in GOOGLE_API_KEY environment")
        return api_key

    # 2. Check GEMINI_API_KEY
    api_key = os.getenv('GEMINI_API_KEY')
    if api_key:
        print("✓ API key found in GEMINI_API_KEY environment")
        return api_key

    # 3. Check skill directory .env
    skill_dir = Path(__file__).parent.parent
    skill_env = skill_dir / '.env'
    if skill_env.exists():
        api_key = load_env_file(skill_env)
        if api_key:
            print(f"✓ API key found in skill directory: {skill_env}")
            return api_key

    # 4. Check project directory .env
    project_dir = skill_dir.parent.parent.parent  # Go up to project root
    project_env = project_dir / '.env'
    if project_env.exists():
        api_key = load_env_file(project_env)
        if api_key:
            print(f"✓ API key found in project directory: {project_env}")
            return api_key

    return None


def load_env_file(env_path: Path) -> Optional[str]:
    """
    Load API key from .env file (GOOGLE_API_KEY or GEMINI_API_KEY)

    Args:
        env_path: Path to .env file

    Returns:
        API key string or None
    """
    try:
        with open(env_path, 'r') as f:
            for line in f:
                line = line.strip()
                if line.startswith(('GOOGLE_API_KEY=', 'GEMINI_API_KEY=')):
                    # Remove quotes if present
                    key = line.split('=', 1)[1].strip()
                    key = key.strip('"').strip("'")
                    return key if key else None
    except Exception as e:
        print(f"Warning: Error reading {env_path}: {e}")

    return None


def generate_image(
    prompt: str,
    api_key: str,
    include_text: bool = False,
    output_path: Optional[str] = None,
    model: str = 'gemini-3-pro-image-preview'
) -> bool:
    """
    Generate image using Gemini API

    Args:
        prompt: Text description of image to generate
        api_key: Gemini API key
        include_text: Whether to include text in response
        output_path: Optional custom output path
        model: Model name to use

    Returns:
        True if successful, False otherwise
    """
    # Build response modalities using Modality enum
    modalities = [Modality.IMAGE]
    if include_text:
        modalities.append(Modality.TEXT)

    print(f"\n🎨 Generating image with Gemini...")
    print(f"   Model: {model}")
    print(f"   Prompt: {prompt}")
    print(f"   Modalities: {', '.join([m.value for m in modalities])}")

    try:
        # Initialize client
        client = genai.Client(api_key=api_key)

        # Generate content with proper enum usage
        config = GenerateContentConfig(
            response_modalities=modalities
        )

        response = client.models.generate_content(
            model=model,
            contents=prompt,
            config=config
        )

        # Process response
        if not response.candidates:
            print("✗ No candidates in response")
            return False

        candidate = response.candidates[0]

        # Save images
        image_count = 0
        for i, part in enumerate(candidate.content.parts):
            if hasattr(part, 'inline_data') and part.inline_data:
                image_count += 1

                # Determine output path
                if output_path:
                    save_path = Path(output_path)
                else:
                    # Default: ./docs/assets/gemini-YYYYMMDD-HHMMSS-N.png
                    timestamp = datetime.now().strftime('%Y%m%d-%H%M%S')
                    assets_dir = Path('./docs/assets')
                    assets_dir.mkdir(parents=True, exist_ok=True)
                    save_path = assets_dir / f'gemini-{timestamp}-{i}.png'

                # Save image
                save_path.parent.mkdir(parents=True, exist_ok=True)
                with open(save_path, 'wb') as f:
                    f.write(part.inline_data.data)

                print(f"✓ Image saved: {save_path}")

        # Print text if included
        if include_text:
            text_parts = [p.text for p in candidate.content.parts if hasattr(p, 'text') and p.text]
            if text_parts:
                print(f"\n📝 Generated text:")
                for text in text_parts:
                    print(f"   {text}")

        if image_count == 0:
            print("✗ No images generated")
            return False

        print(f"\n✓ Successfully generated {image_count} image(s)")
        return True

    except Exception as e:
        print(f"\n✗ Error generating image: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(
        description='Generate images using Gemini 3.0 Preview Image model',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s "A serene mountain landscape"
  %(prog)s "Futuristic city" --output city.png
  %(prog)s "Modern design" --include-text
  %(prog)s "Robot" --model gemini-3-pro-image-preview --output ./my-robot.png
        """
    )

    parser.add_argument(
        'prompt',
        help='Text description of image to generate'
    )

    parser.add_argument(
        '--include-text',
        action='store_true',
        help='Include text description in response'
    )

    parser.add_argument(
        '--output', '-o',
        help='Custom output path (default: ./docs/assets/gemini-TIMESTAMP.png)'
    )

    parser.add_argument(
        '--model',
        default='gemini-3-pro-image-preview',
        help='Model to use (default: gemini-3-pro-image-preview)'
    )

    args = parser.parse_args()

    # Find API key
    print("🔍 Searching for API key...")
    api_key = find_api_key()

    if not api_key:
        print("\n✗ API key not found!")
        print("\nPlease set your API key in one of these locations:")
        print("  1. Environment variable: export GOOGLE_API_KEY='your-key'")
        print("  2. Environment variable: export GEMINI_API_KEY='your-key'")
        print("  3. Skill directory: .claude/skills/gemini-image-gen/.env")
        print("  4. Project root: ./.env")
        print("\nGet your API key at: https://aistudio.google.com/apikey")
        sys.exit(1)

    # Generate image
    success = generate_image(
        prompt=args.prompt,
        api_key=api_key,
        include_text=args.include_text,
        output_path=args.output,
        model=args.model
    )

    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()
