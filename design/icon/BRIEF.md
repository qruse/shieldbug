# Shieldbug icon brief

Shieldbug is a free, open-source daily security check for people who use AI coding agents (Claude Code, Codex, Gemini CLI). It runs on the user's own AI subscription, reads their PC settings, and shows a small local dashboard with a score. Tagline (Korean): "AI 해킹은 AI가 막아야지, 내 토큰으로 쓰는 AI 백신".

## What the owner wants
- The current icon (below) has the right feeling but looks low-effort. Make a proper one.
- Feeling: **friendly and familiar, but a guardian.** Not macho, not corporate, no "cool hacker" look. Think of how Goose or Dots feel: simple, likeable, a little character.
- It should still read as a **security** product.
- Idea worth using: a *shield bug* is a real insect whose back is shaped like a shield. A small, cute bug whose shell is a shield fits the name exactly. Your call on the final concept.

## Current icon (for reference)
```svg
<svg viewBox="0 0 32 32"><path d="M16 3 6 7v8c0 7 4.5 11.5 10 14 5.5-2.5 10-7 10-14V7z" fill="#16a34a"/><path d="M16 9v15M11 13l5 3 5-3M11 19l5-2 5 2" stroke="#fff" stroke-width="1.8" fill="none" stroke-linecap="round"/><circle cx="16" cy="8.5" r="2.2" fill="#fff"/></svg>
```
Brand green today is `#16a34a`. You may adjust the palette slightly, but keep green as the main colour.

## Deliverables (write them in this folder)
1. `icon.svg`: the main app icon, `viewBox="0 0 512 512"`, full colour, works on light and dark backgrounds.
2. `icon-small.svg`: a simplified version that stays clear at 16–32 px (favicon, taskbar). Fewer details, thicker shapes.
3. `icon-mono.svg`: one-colour version using `currentColor`.
4. `preview.html`: shows all three at 512, 128, 64, 32 and 16 px, on a white and on a near-black background.
5. `NOTES.md`: two or three sentences on the concept and the colours.

## Rules
- Hand-written SVG only: no embedded raster images, no external fonts, no text in the icon.
- Keep each SVG under 6 KB, clean paths, no editor metadata.
- Do not touch anything outside this folder.
