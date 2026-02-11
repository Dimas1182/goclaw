---
name: gemini-image-gen
description: Generate images from text prompts using Google Gemini API. Use this skill when the user asks to "generate an image", "create a picture", "draw", or make visual content.
license: MIT
version: 1.0.0
allowed-tools:
  - Bash
  - Read
  - Write
---

# Gemini Image Generation Skill

Use this skill to generate images from text prompts. When activated, you will have access to detailed instructions for using Google Gemini's image generation API.

## How to Use This Skill

1. Call `use_skill` with skill_name="gemini-image-gen"
2. Read the skill content for detailed instructions
3. Use the `run_shell` tool to execute the helper script

**Quick Example**:
```bash
python skills/gemini-image-gen/scripts/generate.py "your image description" --output ./output.png
```

## What This Skill Can Do

- Generate images from text descriptions
- Edit existing images
- Combine multiple images
- Iterative refinement through conversation

## Requirements

- Python package: `pip install google-genai`
- API key: `GOOGLE_API_KEY` or `GEMINI_API_KEY`

## Detailed Instructions
