# Shieldbug icon — round 4 (final)

This is the **final round**. The result becomes the app icon, the favicon, the GitHub avatar and the logo on hslab.space. Take your time and check your own work against the acceptance list at the end before you finish.

Read `BRIEF.md` for the product and the general rules. Images attached: `preview_v3_large.png` (round 3, left = main, right = small) and `preview_v1.png` (round 1).

## What we learned from rounds 1–3
| Round | Kept | Problem |
|---|---|---|
| 1 | Friendly smiling face, approachable | Read as a **leaf** (centre vein), face vanished at small size |
| 2 | Two wing covers + seam: reads as a bug | Looked like the **Android robot** (half-round green head, straight antennae, white dot eyes) |
| 3 | Real shield-bug silhouette, emerald colour, no Android look | **Lost the friendliness** (no smile, tiny eyes); the two dots on the wings read as a **second face**; looks like a generic "software bug/defect" icon |

## The goal of round 4
**Round 3's body and colour, plus round 1's warmth.** A small, likeable guardian beetle whose back is unmistakably a **shield**. In one glance, in this order: *shield → friendly little bug → security*.

## Exact specification

**Canvas.** `viewBox="0 0 512 512"`. Keep all artwork inside a 432 × 432 safe area centred in the canvas (40 px margin). Centre it optically, and make it perfectly left–right symmetric. Mirror the coordinates; do not eyeball them.

**Palette.** Use exactly these colours, no others in the full-colour icon:
- outline / ink `#0f3a2c`
- shell left half `#1fae7a`, shell right half `#169468`: flat two-tone for depth, no gradients
- head `#2fbf8a`
- eye white `#ffffff`, pupil `#0f3a2c`
- dark-background keyline `#eafff5`

**Outline.** One consistent outline weight of **18 px** at 512 for every shape (shell, head, legs, antennae), with round joins and caps. The dark-background keyline is a **30 px** stroke behind the artwork in `#eafff5`, as in rounds 1–3.

**Shell (the hero).** The shell is the shield and the largest shape, about **70% of the total height**. Keep round 3's silhouette: a broad top that curves down to a single point. Keep the centre seam from the top of the shell to the point. **No spots or dots on the shell.**

**Head.** Keep round 3's wide, flat, slightly trapezoid head, but make it **a little larger** (about 26–30% of the shell width taller than round 3) so the face has room. It sits on top of the shell and overlaps it slightly.

**Face (the warmth).** It lives only on the head.
- Two **large eyes**, white with a dark pupil. Each eye is about 34–40 px in diameter at 512. Pupils look slightly upward and inward, toward the viewer, for a friendly, attentive look. Add a tiny white highlight dot in each pupil.
- A **small, gentle smile**: a short arc, 8–10 px stroke, centred under the eyes. Friendly, not goofy.
- No eyebrows, no blush, no other features.

**Antennae.** Two of them, **curved** outward like gentle parentheses, each ending in a small round bulb. Not straight sticks, and not the Android V.

**Legs.** Three short legs per side, peeking out from under the shell edge, mirrored. They are short stubs with rounded ends, the same 18 px stroke. They must not touch the antennae or extend beyond the safe area.

**Must not resemble:** the Android robot, a ladybug with spots, a cockroach, or a leaf.

## Deliverables (this folder; keep all earlier files)
1. `icon-v4.svg`: full colour, as specified.
2. `icon-small-v4.svg` (16–32 px use):
   - Same silhouette and colours, all strokes thickened so nothing is thinner than ~1.5 px at 32 px (≈ 24 px at 512).
   - **Drop the legs and the smile.** Keep shell, seam, head, antennae (thicker, shorter) and the two eyes. Make the eyes even larger relative to the head, simple white with a dark pupil and no highlight.
   - At 16 px the eyes must still be visible as two light dots.
3. `icon-mono-v4.svg`: one colour, using `fill="currentColor"`. Eyes, seam and the gap between head and shell are **cut-outs** (`fill-rule="evenodd"` or a mask), so it works on any background colour. Legs and antennae are included. No smile (it disappears in one colour).
4. Add a "Round 4 (final)" paragraph to `NOTES.md`: the concept in two sentences, plus the palette.

## Acceptance list — check every line before you finish
- [ ] Every SVG parses, uses `viewBox="0 0 512 512"`, has no raster images, text, external references or editor metadata, and is under 6 KB.
- [ ] Artwork is inside the 40 px margin; the left and right halves mirror exactly (state the axis you mirrored around).
- [ ] Only the palette colours above appear in `icon-v4.svg` and `icon-small-v4.svg`.
- [ ] No dots or spots on the shell; the shell is clearly the largest shape.
- [ ] Head outline is not a semicircle; antennae are curved with bulbs.
- [ ] The face has two large eyes with pupils, plus a small smile (main icon only).
- [ ] The small version has no legs and no smile, and its eyes are its largest light shapes.
- [ ] The mono version uses only `currentColor` with real cut-outs.

At the end, print the acceptance list with each item ticked and one line of evidence for each.
