# Shieldbug

**AI 해킹은 AI가 막아야지, 내 토큰으로 쓰는 AI 백신**

A daily security check for people who use AI coding agents. It runs on **the AI you already pay for** (Claude Code, Codex, Gemini CLI, Antigravity, or an API key), so it costs almost nothing. It is open source, so you can read exactly what it does.

> Status: **early alpha (v0.1.0)**. Tested on Windows 11 with Claude Code. macOS, Linux, Codex and Gemini adapters are written but not yet tested. It is a daily check of your setup, not a replacement for antivirus.

[한국어 설명은 아래에 있습니다.](#한국어)

## What it does

1. **Collects facts locally, without AI.** It looks at AI agent settings and MCP servers, plaintext keys and tokens, ports open to the network, OS updates and firewall, and installed tools and extensions.
2. **Redacts them.** Keys, tokens, passwords, your user name, host name, email and public IP are removed. A gate blocks the send if anything key-shaped survives. You can see exactly what was sent (`shieldbug payload`).
3. **Asks your own AI, twice.**
   - The **intel** call searches the web for this week's AI-related attacks. It never sees anything from your machine.
   - The **analysis** call sees the redacted facts with **every tool switched off**, so it can only answer in text.
4. **Shows the result** in a small local dashboard: a score, findings and how to fix each one.

It **never fixes anything by itself** and **never stays resident**. Your OS scheduler runs it once a day, and the dashboard exists only while it is open.

The checklist grows: when a new attack pattern appears, your AI proposes a new check and you approve or reject it.

## Try it

You need [Go 1.27+](https://go.dev/dl/) and at least one of: `claude`, `codex`, `gemini`, `agy`, or an API key in `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` or `GEMINI_API_KEY`.

```bash
go install github.com/qruse/shieldbug@latest
shieldbug doctor      # checks redaction and that the AI cannot use tools
shieldbug check       # runs one check now
shieldbug             # opens the dashboard
```

Other commands: `shieldbug help`. Data lives in `~/.shieldbug/`. `shieldbug uninstall` removes the schedule, shortcuts and all data.

**Cost.** One daily check with Claude Sonnet 5.5 at low effort used about 55 seconds and about $0.11 of usage in our test, counted against your own subscription or key.

## Contributing

Anyone can share cases, propose checklist items and vote. Only reviewed items reach users' machines. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Code: [Apache 2.0](LICENSE). Checklist content: [CC BY 4.0](checklist/LICENSE). "Shieldbug" is a trademark of hslab ([NOTICE](NOTICE)).

---

## 한국어

AI 코딩 도구를 쓰는 사람을 위한 **하루 한 번 보안 점검**입니다. 이미 돈을 내고 쓰는 AI(Claude Code, Codex, Gemini CLI, Antigravity, 또는 API 키)로 돌아가서 비용이 거의 들지 않고, 오픈소스라 무엇을 하는지 전부 볼 수 있습니다.

> 상태: **초기 알파 (v0.1.0)**. Windows 11과 Claude Code에서만 시험했습니다. 백신을 대신하는 게 아니라, 내 PC 설정의 구멍을 매일 찾아 주는 점검입니다.

**하는 일**
1. **AI 없이 PC에서 사실을 모읍니다.** AI 에이전트 설정과 MCP 서버, 평문으로 놓인 키와 토큰, 외부에 열린 포트, OS 업데이트와 방화벽, 설치된 도구를 봅니다.
2. **가립니다.** 키·토큰·비밀번호·사용자 이름·이메일·공인 IP를 지웁니다. 키처럼 생긴 게 남아 있으면 아예 보내지 않습니다. 보낸 내용은 대시보드에서 그대로 볼 수 있습니다.
3. **내 AI에게 두 번 묻습니다.**
   - 뉴스 조사 호출은 이번 주 AI 관련 공격을 웹에서 찾고, 내 PC 정보는 전혀 보지 않습니다.
   - 분석 호출은 가린 자료만 보고, **도구가 하나도 없어서** 글로만 답할 수 있습니다.
4. **결과를 보여 줍니다.** 가벼운 로컬 대시보드에 점수, 발견 사항, 고치는 법이 나옵니다.

**절대 스스로 고치지 않고, 계속 떠 있지도 않습니다.** 점검목록은 새 공격이 나올 때마다 AI가 항목을 제안하고 내가 승인하면서 자랍니다.

설치와 사용법은 위 영어 섹션의 명령과 같습니다. 참여 방법은 [CONTRIBUTING.md](CONTRIBUTING.md)를 보세요.
