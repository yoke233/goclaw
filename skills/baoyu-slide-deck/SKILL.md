---
name: baoyu-slide-deck
description: Generates professional slide deck with images and optional PPT/PDF export. Creates outlines, style instructions, generates individual slide images, and merges to PowerPoint/PDF. Use when user asks to "create slides", "make a presentation", "generate deck", "slide deck", or "PPT".
---

# Slide Deck Generator

Transform content into professional slide deck images and export to PowerPoint/PDF formats.

## Primary Goal

**Your objective is to help users create presentation slides from content.**

The complete workflow produces:
1. **Slide images** (PNG) - Individual slides for social media or web sharing
2. **PowerPoint file** (PPTX) - For editing in PowerPoint/Keynote
3. **PDF document** - For easy sharing and viewing

**IMPORTANT**:
- Always attempt to generate PPTX and PDF files if Node.js is available
- If PPT/PDF generation fails, still provide the PNG images with clear instructions
- Never fail the entire task just because PPT export failed - the PNG images are valuable on their own

## Usage

**IMPORTANT**: This is a SKILL, not a command-line tool. When activated via the `use_skill` tool, follow this workflow:

### Input Options (from user request):

- **Content source**: User provides content directly or as a file path
- **Style preference**: `--style <name>` (optional, e.g., blueprint, minimal, corporate)
- **Target audience**: `--audience <type>` (optional, e.g., beginners, experts, executives)
- **Language**: `--lang <code>` (optional, e.g., en, zh, ja)
- **Slide count**: `--slides <number>` (optional, 8-25 recommended)
- **Outline only**: `--outline-only` (skip image generation)
- **Prompts only**: `--prompts-only` (generate prompts without images)
- **Images only**: `--images-only` (generate images from existing prompts)

### How to Use:

1. **User says**: "Create a PPT about [topic]" or "Generate slides from [content]"
2. **This skill is activated** automatically via `use_skill` tool
3. **Follow the workflow** in this SKILL.md file to:
   - Search for information (if needed)
   - Generate outline
   - Create slide prompts
   - Generate slide images
   - Merge to PPTX/PDF (if possible)

### DO NOT:

- ❌ Try to execute `/baoyu-slide-deck` as a command (it doesn't exist)
- ❌ Use `exec` tool to run this skill
- ❌ Look for binary executables named `baoyu-slide-deck`

### Example Workflow:

```
User: "Create a presentation about Go programming"
→ You: Activate this skill via use_skill (automatic)
→ You: Ask user for content or search web for information
→ You: Generate outline based on research
→ You: Create slide prompts with style instructions
→ You: Generate individual slide images
→ You: Merge images into PPTX/PDF (if Node.js available)
→ You: Provide final output to user
```

## Script Directory

**Agent Execution Instructions**:
1. Determine this SKILL.md file's directory path as `SKILL_DIR`
2. Script path = `${SKILL_DIR}/scripts/<script-name>.ts`

| Script | Purpose |
|--------|---------|
| `scripts/merge-to-pptx.ts` | Merge slides into PowerPoint |
| `scripts/merge-to-pdf.ts` | Merge slides into PDF |

## Options

| Option | Description |
|--------|-------------|
| `--style <name>` | Visual style: preset name, `custom`, or custom style name |
| `--audience <type>` | Target: beginners, intermediate, experts, executives, general |
| `--lang <code>` | Output language (en, zh, ja, etc.) |
| `--slides <number>` | Target slide count (8-25 recommended, max 30) |
| `--outline-only` | Generate outline only, skip image generation |
| `--prompts-only` | Generate outline + prompts, skip images |
| `--images-only` | Generate images from existing prompts directory |
| `--regenerate <N>` | Regenerate specific slide(s): `--regenerate 3` or `--regenerate 2,5,8` |

**Slide Count by Content Length**:
| Content | Slides |
|---------|--------|
| < 1000 words | 5-10 |
| 1000-3000 words | 10-18 |
| 3000-5000 words | 15-25 |
| > 5000 words | 20-30 (consider splitting) |

## Style System

### Presets

| Preset | Dimensions | Best For |
|--------|------------|----------|
| `blueprint` (Default) | grid + cool + technical + balanced | Architecture, system design |
| `chalkboard` | organic + warm + handwritten + balanced | Education, tutorials |
| `corporate` | clean + professional + geometric + balanced | Investor decks, proposals |
| `minimal` | clean + neutral + geometric + minimal | Executive briefings |
| `sketch-notes` | organic + warm + handwritten + balanced | Educational, tutorials |
| `watercolor` | organic + warm + humanist + minimal | Lifestyle, wellness |
| `dark-atmospheric` | clean + dark + editorial + balanced | Entertainment, gaming |
| `notion` | clean + neutral + geometric + dense | Product demos, SaaS |
| `bold-editorial` | clean + vibrant + editorial + balanced | Product launches, keynotes |
| `editorial-infographic` | clean + cool + editorial + dense | Tech explainers, research |
| `fantasy-animation` | organic + vibrant + handwritten + minimal | Educational storytelling |
| `intuition-machine` | clean + cool + technical + dense | Technical docs, academic |
| `pixel-art` | pixel + vibrant + technical + balanced | Gaming, developer talks |
| `scientific` | clean + cool + technical + dense | Biology, chemistry, medical |
| `vector-illustration` | clean + vibrant + humanist + balanced | Creative, children's content |
| `vintage` | paper + warm + editorial + balanced | Historical, heritage |

### Style Dimensions

| Dimension | Options | Description |
|-----------|---------|-------------|
| **Texture** | clean, grid, organic, pixel, paper | Visual texture and background treatment |
| **Mood** | professional, warm, cool, vibrant, dark, neutral | Color temperature and palette style |
| **Typography** | geometric, humanist, handwritten, editorial, technical | Headline and body text styling |
| **Density** | minimal, balanced, dense | Information density per slide |

Full specs: `references/dimensions/*.md`

### Auto Style Selection

| Content Signals | Preset |
|-----------------|--------|
| tutorial, learn, education, guide, beginner | `sketch-notes` |
| classroom, teaching, school, chalkboard | `chalkboard` |
| architecture, system, data, analysis, technical | `blueprint` |
| creative, children, kids, cute | `vector-illustration` |
| briefing, academic, research, bilingual | `intuition-machine` |
| executive, minimal, clean, simple | `minimal` |
| saas, product, dashboard, metrics | `notion` |
| investor, quarterly, business, corporate | `corporate` |
| launch, marketing, keynote, magazine | `bold-editorial` |
| entertainment, music, gaming, atmospheric | `dark-atmospheric` |
| explainer, journalism, science communication | `editorial-infographic` |
| story, fantasy, animation, magical | `fantasy-animation` |
| gaming, retro, pixel, developer | `pixel-art` |
| biology, chemistry, medical, scientific | `scientific` |
| history, heritage, vintage, expedition | `vintage` |
| lifestyle, wellness, travel, artistic | `watercolor` |
| Default | `blueprint` |

## Design Philosophy

Decks designed for **reading and sharing**, not live presentation:
- Each slide self-explanatory without verbal commentary
- Logical flow when scrolling
- All necessary context within each slide
- Optimized for social media sharing

See `references/design-guidelines.md` for:
- Audience-specific principles
- Visual hierarchy
- Content density guidelines
- Color and typography selection
- Font recommendations

See `references/layouts.md` for layout options.

## File Management

### Output Directory

```
slide-deck/{topic-slug}/
├── source-{slug}.{ext}
├── outline.md
├── prompts/
│   └── 01-slide-cover.md, 02-slide-{slug}.md, ...
├── 01-slide-cover.png, 02-slide-{slug}.png, ...
├── {topic-slug}.pptx
└── {topic-slug}.pdf
```

**Slug**: Extract topic (2-4 words, kebab-case). Example: "Introduction to Machine Learning" → `intro-machine-learning`

**Conflict Handling**: See Step 1.3 for existing content detection and user options.

## Error Handling & Fallback Strategies

### File Write Failures

When `write_file` tool returns errors like "access to path ... is not allowed":

**Detection Patterns**:
- `access to path .* is not allowed`
- `permission denied`
- `cannot create file`
- `no such file or directory`

**Fallback Strategy**:
1. **First Retry**: Try with explicit relative path (`./filename`)
2. **Second Retry**: Try with `/tmp/` prefix for temporary files
3. **Final Fallback**: Output content directly to console with copy instructions

**Console Output Format**:
```
⚠️ File Write Failed

Unable to save to: `{filename}`

**Reason**: {error message}

---
**Content** (copy and save manually):
```markdown
{file content}
```
---

**Suggested Actions**:
1. Create directory: `mkdir -p {directory}`
2. Check permissions: `ls -la {directory}`
3. Try different location: `/tmp/{filename}`
```

### Image Generation Failures

When image generation fails:

**Common Issues**:
- API rate limits
- Invalid prompts
- Network timeouts
- Service unavailable

**Handling Steps**:
1. Log the specific slide number that failed
2. Save the failed prompt for review
3. Continue with remaining slides
4. Provide summary of successes/failures
5. Suggest retry options for failed slides

**Progress Reporting Example**:
```
Image Generation Progress:
✅ Slide 1/10 - Cover
✅ Slide 2/10 - Introduction
❌ Slide 3/10 - Overview (API timeout)
✅ Slide 4/10 - Details
...
✅ Slide 10/10 - Conclusion

Summary: 9/10 slides generated successfully
Retry failed: --regenerate 3
```

### Web/Browser Tool Failures

When web scraping or browser automation fails:

**Fallback Options**:
1. Ask user to provide content directly
2. Try alternative URL (if available)
3. Use cached/summary version
4. Proceed with available information

## Language Handling

**Detection Priority**:
1. `--lang` flag (explicit)
2. EXTEND.md `language` setting
3. User's conversation language (input language)
4. Source content language

**Rule**: ALL responses use user's preferred language:
- Questions and confirmations
- Progress reports
- Error messages
- Completion summaries

Technical terms (style names, file paths, code) remain in English.

## Workflow

Copy this checklist and check off items as you complete them:

```
Slide Deck Progress:
- [ ] Step 1: Setup & Analyze
  - [ ] 1.1 Load preferences
  - [ ] 1.2 Analyze content
  - [ ] 1.3 Check existing ⚠️ REQUIRED
- [ ] Step 2: Confirmation ⚠️ REQUIRED (Round 1, optional Round 2)
- [ ] Step 3: Generate outline
- [ ] Step 4: Review outline (conditional)
- [ ] Step 5: Generate prompts
- [ ] Step 6: Review prompts (conditional)
- [ ] Step 7: Generate images
- [ ] Step 8: Merge to PPTX/PDF
- [ ] Step 9: Output summary
```

### Flow

```
Input → Preferences → Analyze → [Check Existing?] → Confirm (1-2 rounds) → Outline → [Review Outline?] → Prompts → [Review Prompts?] → Images → Merge → Complete
```

### Step 1: Setup & Analyze

**1.1 Load Preferences (EXTEND.md)**

Use Bash to check EXTEND.md existence (priority order):

```bash
# Check project-level first
test -f .baoyu-skills/baoyu-slide-deck/EXTEND.md && echo "project"

# Then user-level (cross-platform: $HOME works on macOS/Linux/WSL)
test -f "$HOME/.baoyu-skills/baoyu-slide-deck/EXTEND.md" && echo "user"
```

┌──────────────────────────────────────────────────┬───────────────────┐
│                       Path                       │     Location      │
├──────────────────────────────────────────────────┼───────────────────┤
│ .baoyu-skills/baoyu-slide-deck/EXTEND.md         │ Project directory │
├──────────────────────────────────────────────────┼───────────────────┤
│ $HOME/.baoyu-skills/baoyu-slide-deck/EXTEND.md   │ User home         │
└──────────────────────────────────────────────────┴───────────────────┘

**When EXTEND.md Found** → Read, parse, **output summary to user**:

```
📋 Loaded preferences from [full path]
├─ Style: [preset/custom name]
├─ Audience: [audience or "auto-detect"]
├─ Language: [language or "auto-detect"]
└─ Review: [enabled/disabled]
```

**When EXTEND.md Not Found** → First-time setup using AskUserQuestion or proceed with defaults.

**EXTEND.md Supports**: Preferred style | Custom dimensions | Default audience | Language preference | Review preference

Schema: `references/config/preferences-schema.md`

**1.2 Analyze Content**

1. Save source content (if pasted, save as `source.md`)
   - **Backup rule**: If `source.md` exists, rename to `source-backup-YYYYMMDD-HHMMSS.md`
2. Follow `references/analysis-framework.md` for content analysis
3. Analyze content signals for style recommendations
4. Detect source language
5. Determine recommended slide count
6. Generate topic slug from content

**1.3 Check Existing Content** ⚠️ REQUIRED

**MUST execute before proceeding to Step 2.**

Use Bash to check if output directory exists:

```bash
test -d "slide-deck/{topic-slug}" && echo "exists"
```

**If directory exists**, use AskUserQuestion:

```
header: "Existing"
question: "Existing content found. How to proceed?"
options:
  - label: "Regenerate outline"
    description: "Keep images, regenerate outline only"
  - label: "Regenerate images"
    description: "Keep outline, regenerate images only"
  - label: "Backup and regenerate"
    description: "Backup to {slug}-backup-{timestamp}, then regenerate all"
  - label: "Exit"
    description: "Cancel, keep existing content unchanged"
```

**Save to `analysis.md`** with:
- Topic, audience, content signals
- Recommended style (based on Auto Style Selection)
- Recommended slide count
- Language detection

### Step 2: Confirmation ⚠️ REQUIRED

**Two-round confirmation**: Round 1 always, Round 2 only if "Custom dimensions" selected.

**Language**: Use user's input language or saved language preference.

**Display summary**:
- Content type + topic identified
- Language: [from EXTEND.md or detected]
- **Recommended style**: [preset] (based on content signals)
- **Recommended slides**: [N] (based on content length)

#### Round 1 (Always)

**Use AskUserQuestion** for all 5 questions:

**Question 1: Style**
```
header: "Style"
question: "Which visual style for this deck?"
options:
  - label: "{recommended_preset} (Recommended)"
    description: "Best match based on content analysis"
  - label: "{alternative_preset}"
    description: "[alternative style description]"
  - label: "Custom dimensions"
    description: "Choose texture, mood, typography, density separately"
```

**Question 2: Audience**
```
header: "Audience"
question: "Who is the primary reader?"
options:
  - label: "General readers (Recommended)"
    description: "Broad appeal, accessible content"
  - label: "Beginners/learners"
    description: "Educational focus, clear explanations"
  - label: "Experts/professionals"
    description: "Technical depth, domain knowledge"
  - label: "Executives"
    description: "High-level insights, minimal detail"
```

**Question 3: Slide Count**
```
header: "Slides"
question: "How many slides?"
options:
  - label: "{N} slides (Recommended)"
    description: "Based on content length"
  - label: "Fewer ({N-3} slides)"
    description: "More condensed, less detail"
  - label: "More ({N+3} slides)"
    description: "More detailed breakdown"
```

**Question 4: Review Outline**
```
header: "Outline"
question: "Review outline before generating prompts?"
options:
  - label: "Yes, review outline (Recommended)"
    description: "Review slide titles and structure"
  - label: "No, skip outline review"
    description: "Proceed directly to prompt generation"
```

**Question 5: Review Prompts**
```
header: "Prompts"
question: "Review prompts before generating images?"
options:
  - label: "Yes, review prompts (Recommended)"
    description: "Review image generation prompts"
  - label: "No, skip prompt review"
    description: "Proceed directly to image generation"
```

#### Round 2 (Only if "Custom dimensions" selected)

**Use AskUserQuestion** for all 4 dimensions:

**Question 1: Texture**
```
header: "Texture"
question: "Which visual texture?"
options:
  - label: "clean"
    description: "Pure solid color, no texture"
  - label: "grid"
    description: "Subtle grid overlay, technical"
  - label: "organic"
    description: "Soft textures, hand-drawn feel"
  - label: "pixel"
    description: "Chunky pixels, 8-bit aesthetic"
```
(Note: "paper" available via Other)

**Question 2: Mood**
```
header: "Mood"
question: "Which color mood?"
options:
  - label: "professional"
    description: "Cool-neutral, navy/gold"
  - label: "warm"
    description: "Earth tones, friendly"
  - label: "cool"
    description: "Blues, grays, analytical"
  - label: "vibrant"
    description: "High saturation, bold"
```
(Note: "dark", "neutral" available via Other)

**Question 3: Typography**
```
header: "Typography"
question: "Which typography style?"
options:
  - label: "geometric"
    description: "Modern sans-serif, clean"
  - label: "humanist"
    description: "Friendly, readable"
  - label: "handwritten"
    description: "Marker/brush, organic"
  - label: "editorial"
    description: "Magazine style, dramatic"
```
(Note: "technical" available via Other)

**Question 4: Density**
```
header: "Density"
question: "Information density?"
options:
  - label: "balanced (Recommended)"
    description: "2-3 key points per slide"
  - label: "minimal"
    description: "One focus point, maximum whitespace"
  - label: "dense"
    description: "Multiple data points, compact"
```

**After Round 2**: Store custom dimensions as the style configuration.

**After Confirmation**:
1. Update `analysis.md` with confirmed preferences
2. Store `skip_outline_review` flag from Question 4
3. Store `skip_prompt_review` flag from Question 5
4. → Step 3

### Step 3: Generate Outline

Create outline using the confirmed style from Step 2.

**Style Resolution**:
- If preset selected → Read `references/styles/{preset}.md`
- If custom dimensions → Read dimension files from `references/dimensions/` and combine

**Generate**:
1. Follow `references/outline-template.md` for structure
2. Build STYLE_INSTRUCTIONS from style or dimensions
3. Apply confirmed audience, language, slide count
4. Save as `outline.md`

**After generation**:
- If `--outline-only`, stop here
- If `skip_outline_review` is true → Skip Step 4, go to Step 5
- If `skip_outline_review` is false → Continue to Step 4

### Step 4: Review Outline (Conditional)

**Skip this step** if user selected "No, skip outline review" in Step 2.

**Purpose**: Review outline structure before prompt generation.

**Language**: Use user's input language or saved language preference.

**Display**:
- Total slides: N
- Style: [preset name or "custom: texture+mood+typography+density"]
- Slide-by-slide summary table:

```
| # | Title | Type | Layout |
|---|-------|------|--------|
| 1 | [title] | Cover | title-hero |
| 2 | [title] | Content | [layout] |
| 3 | [title] | Content | [layout] |
| ... | ... | ... | ... |
```

**Use AskUserQuestion**:
```
header: "Confirm"
question: "Ready to generate prompts?"
options:
  - label: "Yes, proceed (Recommended)"
    description: "Generate image prompts"
  - label: "Edit outline first"
    description: "I'll modify outline.md before continuing"
  - label: "Regenerate outline"
    description: "Create new outline with different approach"
```

**After response**:
1. If "Edit outline first" → Inform user to edit `outline.md`, ask again when ready
2. If "Regenerate outline" → Back to Step 3
3. If "Yes, proceed" → Continue to Step 5

### Step 5: Generate Prompts

1. Read `references/base-prompt.md`
2. For each slide in outline:
   - Extract STYLE_INSTRUCTIONS from outline (not from style file again)
   - Add slide-specific content
   - If `Layout:` specified, include layout guidance from `references/layouts.md`
3. Save to `prompts/` directory
   - **Backup rule**: If prompt file exists, rename to `prompts/NN-slide-{slug}-backup-YYYYMMDD-HHMMSS.md`

**After generation**:
- If `--prompts-only`, stop here and output prompt summary
- If `skip_prompt_review` is true → Skip Step 6, go to Step 7
- If `skip_prompt_review` is false → Continue to Step 6

### Step 6: Review Prompts (Conditional)

**Skip this step** if user selected "No, skip prompt review" in Step 2.

**Purpose**: Review prompts before image generation.

**Language**: Use user's input language or saved language preference.

**Display**:
- Total prompts: N
- Style: [preset name or custom dimensions]
- Prompt list:

```
| # | Filename | Slide Title |
|---|----------|-------------|
| 1 | 01-slide-cover.md | [title] |
| 2 | 02-slide-xxx.md | [title] |
| ... | ... | ... |
```

- Path to prompts directory: `prompts/`

**Use AskUserQuestion**:
```
header: "Confirm"
question: "Ready to generate slide images?"
options:
  - label: "Yes, proceed (Recommended)"
    description: "Generate all slide images"
  - label: "Edit prompts first"
    description: "I'll modify prompts before continuing"
  - label: "Regenerate prompts"
    description: "Create new prompts with different approach"
```

**After response**:
1. If "Edit prompts first" → Inform user to edit prompts, ask again when ready
2. If "Regenerate prompts" → Back to Step 5
3. If "Yes, proceed" → Continue to Step 7

### Step 7: Generate Images

**For `--images-only`**: Start here with existing prompts.

**For `--regenerate N`**: Only regenerate specified slide(s).

**Standard flow**:
1. Select available image generation skill
2. Generate session ID: `slides-{topic-slug}-{timestamp}`
3. For each slide:
   - **Backup rule**: If image file exists, rename to `NN-slide-{slug}-backup-YYYYMMDD-HHMMSS.png`
   - Generate image sequentially with same session ID
4. Report progress: "Generated X/N" (in user's language)
5. Auto-retry once on failure before reporting error

**⚠️ Error Handling for File Operations**:

If `write_file` tool fails with "access to path ... is not allowed":

**Immediate Actions**:
1. **Output to Console**: Display the content directly to the user with clear formatting
2. **Use Code Blocks**: Wrap file content in triple backticks with language identifier
3. **Provide Copy Instructions**: Tell user they can copy the content manually

**Example Response Format**:
```
# File Write Failed

Unable to write to `slide-deck/topic/outline.md` due to permission restrictions.

## Outline Content (copy and save manually):

```markdown
# Slide Deck Outline
...
```

## Suggested Actions:
1. Copy the content above and save it manually
2. Check if the output directory exists: `mkdir -p slide-deck/topic`
3. Verify write permissions for the current directory
```

**Retry Strategy**:
- First attempt: Try writing with relative path `./filename`
- Second attempt: Try writing with absolute path
- If both fail: Output to console and provide manual save instructions

**Detection Pattern**:
Watch for error messages containing:
- "access to path ... is not allowed"
- "permission denied"
- "cannot create file"

### Step 8: Merge to PPTX and PDF

**IMPORTANT**: This step requires Node.js and the slide-merge scripts. If these are not available, provide alternative instructions to the user.

**Check prerequisites first**:
```bash
# Check if Node.js is available
node --version || nodejs --version
# Check if bun is available (optional, but faster)
bun --version
```

**If Node.js is available**, try the merge scripts:

```bash
# First, determine SKILL_DIR by checking where this SKILL.md file is located
# Then run the merge scripts

# Method 1: Using bun (if available, faster)
if command -v bun &> /dev/null; then
    bun ${SKILL_DIR}/scripts/merge-to-pptx.ts <slide-deck-dir>
    bun ${SKILL_DIR}/scripts/merge-to-pdf.ts <slide-deck-dir>
# Method 2: Using npx (slower, but works without bun)
elif command -v npx &> /dev/null; then
    npx -y bun ${SKILL_DIR}/scripts/merge-to-pptx.ts <slide-deck-dir>
    npx -y bun ${SKILL_DIR}/scripts/merge-to-pdf.ts <slide-deck-dir>
# Method 3: Using ts-node (if available)
elif command -v ts-node &> /dev/null; then
    ts-node ${SKILL_DIR}/scripts/merge-to-pptx.ts <slide-deck-dir>
    ts-node ${SKILL_DIR}/scripts/merge-to-pdf.ts <slide-deck-dir>
fi
```

**If merge scripts fail or are not available**:

1. **Inform the user** that the automatic merge failed
2. **Provide the slide images** as the final output
3. **Give manual instructions** for creating PPT:

```
Note: Automatic PPT/PDF generation failed. However, all slide images have been generated successfully!

Your slides are ready at: slide-deck/{topic-slug}/

To create a PPT manually:
1. Open PowerPoint or Keynote
2. Create a new presentation with 16:9 aspect ratio
3. Import the generated PNG images in order:
   - 01-slide-cover.png (Title slide)
   - 02-slide-*.png (Content slides)
   - NN-slide-back-cover.png (Closing slide)
4. Save as PPTX

To create a PDF:
1. Open the PPTX in PowerPoint/Keynote
2. Export as PDF
```

**Success response format** (when merge succeeds):
```
Slide Deck Complete!

Topic: [topic]
Style: [preset name]
Location: slide-deck/[topic-slug]/
Slides: N total

Generated files:
✅ [N] PNG images - Individual slides
✅ [topic-slug].pptx - PowerPoint presentation
✅ [topic-slug].pdf - PDF document

You can now:
- Open the PPTX in PowerPoint/Keynote for editing
- Share the PDF for viewing
- Use individual PNG images for social media
```

### Step 9: Output Summary

**Language**: Use user's input language or saved language preference.

```
Slide Deck Complete!

Topic: [topic]
Style: [preset name or custom dimensions]
Location: [directory path]
Slides: N total

- 01-slide-cover.png - Cover
- 02-slide-intro.png - Content
- ...
- {NN}-slide-back-cover.png - Back Cover

Outline: outline.md
PPTX: {topic-slug}.pptx
PDF: {topic-slug}.pdf
```

## Partial Workflows

| Option | Workflow |
|--------|----------|
| `--outline-only` | Steps 1-3 only (stop after outline) |
| `--prompts-only` | Steps 1-5 (generate prompts, skip images) |
| `--images-only` | Skip to Step 7 (requires existing prompts/) |
| `--regenerate N` | Regenerate specific slide(s) only |

### Using `--prompts-only`

Generate outline and prompts without images:

```bash
/baoyu-slide-deck content.md --prompts-only
```

Output: `outline.md` + `prompts/*.md` ready for review/editing.

### Using `--images-only`

Generate images from existing prompts (starts at Step 7):

```bash
/baoyu-slide-deck slide-deck/topic-slug/ --images-only
```

Prerequisites:
- `prompts/` directory with slide prompt files
- `outline.md` with style information

### Using `--regenerate`

Regenerate specific slides:

```bash
# Single slide
/baoyu-slide-deck slide-deck/topic-slug/ --regenerate 3

# Multiple slides
/baoyu-slide-deck slide-deck/topic-slug/ --regenerate 2,5,8
```

Flow:
1. Read existing prompts for specified slides
2. Regenerate images only for those slides
3. Regenerate PPTX/PDF

## Slide Modification

### Quick Reference

| Action | Command | Manual Steps |
|--------|---------|--------------|
| **Edit** | `--regenerate N` | **Update prompt file FIRST** → Regenerate image → Regenerate PDF |
| **Add** | Manual | Create prompt → Generate image → Renumber subsequent → Update outline → Regenerate PDF |
| **Delete** | Manual | Remove files → Renumber subsequent → Update outline → Regenerate PDF |

### Edit Single Slide

1. **Update prompt file FIRST** in `prompts/NN-slide-{slug}.md`
2. Run: `/baoyu-slide-deck <dir> --regenerate N`
3. Or manually regenerate image + PDF

**IMPORTANT**: When updating slides, ALWAYS update the prompt file (`prompts/NN-slide-{slug}.md`) FIRST before regenerating. This ensures changes are documented and reproducible.

### Add New Slide

1. Create prompt at position: `prompts/NN-slide-{new-slug}.md`
2. Generate image using same session ID
3. **Renumber**: Subsequent files NN+1 (slugs unchanged)
4. Update `outline.md`
5. Regenerate PPTX/PDF

### Delete Slide

1. Remove `NN-slide-{slug}.png` and `prompts/NN-slide-{slug}.md`
2. **Renumber**: Subsequent files NN-1 (slugs unchanged)
3. Update `outline.md`
4. Regenerate PPTX/PDF

### File Naming

Format: `NN-slide-[slug].png`
- `NN`: Two-digit sequence (01, 02, ...)
- `slug`: Kebab-case from content (2-5 words, unique)

**Renumbering Rule**: Only NN changes, slugs remain unchanged.

See `references/modification-guide.md` for complete details.

## References

| File | Content |
|------|---------|
| `references/analysis-framework.md` | Content analysis for presentations |
| `references/outline-template.md` | Outline structure and format |
| `references/modification-guide.md` | Edit, add, delete slide workflows |
| `references/content-rules.md` | Content and style guidelines |
| `references/design-guidelines.md` | Audience, typography, colors, visual elements |
| `references/layouts.md` | Layout options and selection tips |
| `references/base-prompt.md` | Base prompt for image generation |
| `references/dimensions/*.md` | Dimension specifications (texture, mood, typography, density) |
| `references/dimensions/presets.md` | Preset → dimension mapping |
| `references/styles/<style>.md` | Full style specifications (legacy) |
| `references/config/preferences-schema.md` | EXTEND.md structure |

## Notes

- Image generation: 10-30 seconds per slide
- Auto-retry once on generation failure
- Use stylized alternatives for sensitive public figures
- Maintain style consistency via session ID
- **Step 2 confirmation required** - do not skip (style, audience, slides, outline review, prompt review)
- **Step 4 conditional** - only if user requested outline review in Step 2
- **Step 6 conditional** - only if user requested prompt review in Step 2

## Extension Support

Custom configurations via EXTEND.md. See **Step 1.1** for paths and supported options.
