You are the lead planner for a new open-source project called **Shieldbug**. Write the complete plan to `planning/PLAN.md` (relative to the current folder) (Korean, concise, decisive; tables welcome). Do not write application code. Use WebSearch/WebFetch to check facts you cite, and put a source URL next to every factual claim about real incidents, products or prices.

## What the owner wants (HSLab, an individual AI researcher in Korea)

- Article/project title (fixed, do not change): "AI 해킹은 AI가 막아야지, 내 토큰으로 쓰는 AI 백신"
- AI-driven hacking is exploding (the owner specifically mentions "the Nike/니케 case" — find out what this refers to in recent news, 2026, and summarise it with sources; if you cannot confirm it, say so plainly instead of guessing).
- Classic antivirus reacts too slowly to new AI-era attack patterns. Instead, the user's own AI models should read new issues every day, adapt flexibly, and maintain a living security checklist.
- The owner does not sell security expertise. The point: anyone can run a personal, customised security routine almost for free with the AI subscription/tokens they already pay for. Goal: blunt AI misuse and spread the benefit widely.
- Defaults: one security check per day; models at the "fast/cheap" tier: GPT-6.1 Terra (Codex CLI), Claude Sonnet 5.5 (Claude Code), Gemini 3.8 Flash (Gemini/Antigravity CLI). Users can customise model, reasoning effort, check types, schedule.
- Distribution: a separate public GitHub repo. A user tells Claude Code / Codex / similar: "GitHub의 Shieldbug 보고 가져와서 보안 시스템 구축해줘" and it installs and opens a localhost security dashboard. One sentence, one command. Must adapt to whichever AI CLI(s) the user has.
- Community: hslab.space gets a community link and a home-page section where people post defence success stories and hacking cases, building a global AI-security commons.
- The dashboard must be **very light**: minimal memory, no heavy framework. People hate heavy security tools. The owner asked "Rust is popular lately, is it good?" — answer this with a reasoned comparison.

## Facts about the build machine (you may rely on these)

- Windows 11; Python 3 available; git and gh available; Go 1.27.4 is installed at C:\tools\go; no Rust or Node on PATH (they can be installed if your plan needs them).
- A first Go draft exists in the current folder (config.go, redact.go, checklist/base.json). Read it, reuse what is good, and say plainly if the shape should change.
- Claude Code CLI: `claude -p --model <id> --effort <low|medium|high|xhigh|max> --permission-mode <mode> --allowedTools ...`
- Codex CLI: `codex exec -m <model> -c model_reasoning_effort="..." -` (prompt on stdin), sandbox modes exist (read-only etc.)
- Antigravity CLI (Gemini): `agy -p "<prompt>" --model <id> --print-timeout 600s`

## The plan must decide and specify

1. One-paragraph positioning and an honest list of what Shieldbug is NOT (not an antivirus replacement, not an EDR, no guarantees).
2. Threat model for an individual developer/power user in the AI era (prompt injection via files/web, malicious MCP servers / agent skills / plugins / hooks, poisoned packages, leaked API keys and tokens, over-permissive agent settings, exposed local services, AI-made phishing, etc.). Prioritise what can actually be checked locally.
3. The daily check pipeline, concretely: what is collected locally, how secrets are redacted BEFORE anything is sent to a model, what prompt/checklist the model gets, how it fetches today's new threats, how results are scored and stored. The security tool itself must be safe: model runs read-only / report-only, never auto-fixes without explicit user approval, no dangerous flags.
4. The living checklist: file format, versioning, how the model proposes additions from new advisories, how the user approves them, how community checklists could be shared later.
5. Adapter design for Claude Code / Codex / Gemini(Antigravity) and how to add more; auto-detection of what the user has; config file with defaults and every customisable field.
6. Scheduling without a resident daemon if possible (OS scheduler runs `shieldbug check` once a day; the dashboard server only runs while open) — or argue otherwise with numbers.
7. **Framework review by user segment (the owner's top concern — spend real effort here).** First list the realistic user segments and what each already has installed, for example: Windows non-developer who only uses the Claude desktop app or ChatGPT/Codex desktop app (no terminal, no Node/Python); developer with Claude Code / Codex CLI / Gemini CLI on Windows, macOS or Linux (Node or a native installer); Linux server/VPS user over SSH (no browser on the box); company laptop without admin rights, AV that blocks unsigned binaries, proxy; Apple Silicon Mac; WSL users; people who only want a phone notification. Then evaluate at least these delivery shapes against every segment: (a) single native binary (Go or Rust) from GitHub Releases, (b) Node package run via `npx` (Claude Code/Codex users often have Node), (c) Python stdlib script, (d) Deno/Bun single-file, (e) PowerShell + bash scripts that generate a static HTML report (no server at all), (f) Tauri tray app, (g) Electron (as the heavy baseline), (h) "agent-native": a Claude Code skill/plugin, Codex AGENTS.md routine, Gemini extension, using the agent's own scheduler (e.g. Claude desktop scheduled tasks, cloud routines, Codex automations) with a static dashboard file — i.e. no runtime of our own, (i) Docker. Score each on: idle RAM, disk, install friction per segment, unsigned-binary/SmartScreen/Gatekeeper problems, antivirus false positives, update path, how easily an AI agent can install it from a one-sentence request, how easily users and the AI can customise it, auditability (can a user read the whole thing?), cross-platform effort for one maintainer. Recommend a primary shape plus a fallback, possibly a hybrid (e.g. core logic in one portable form, thin per-OS scheduler glue). Be explicit about Rust: when it is worth it and when it is not for this project.
8. **Tech stack decision** with a comparison table (Rust, Go, Python stdlib, Node, a static HTML file + tiny server): idle RAM, binary/download size, install friction on Windows/macOS/Linux when an AI agent installs it from a fresh clone, build/CI complexity, cross-platform release. Pick one and justify. Include the dashboard front-end approach (plain HTML/CSS/JS, no framework unless justified) and data store (JSON/SQLite).
9. Dashboard: screens and exactly what is shown (today's status, findings with severity and fix steps, trend, checklist, settings, run-now button, AI usage/cost per run).
10. The one-line install flow: repo files that make AI agents install it correctly (README section, AGENTS.md / CLAUDE.md instructions), what the agent must ask the user, what it must never do.
11. Repo layout (files and folders), licence suggestion, privacy statement.
12. hslab.space side: project page outline (KO/EN), community section design (submission → moderation → publish), what to build first.
13. MVP for TODAY (must end with a working localhost dashboard showing a real check run on this machine), then milestones after.
14. Risks and open questions for the owner (keep short).
