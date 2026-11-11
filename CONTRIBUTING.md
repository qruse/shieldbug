# Contributing to Shieldbug

**Anyone can propose. Only reviewed changes reach users.** Shieldbug runs on people's own machines, so a bad change here would be an attack on them. The rules below exist for that reason.

[한국어 안내는 아래에 있습니다.](#한국어)

## Ways to help

| What | How | Needs the CLA? |
|---|---|---|
| Share a case: an AI-related attack you saw, or one you stopped | Issue form "Case report" | No (CC BY 4.0) |
| Propose a checklist item | Issue form "Checklist proposal", source link required | No (CC BY 4.0) |
| Vote on checklist items | 👍 / 👎 on the proposal issue, or "helpful / false alarm" in the dashboard (opt-in) | No |
| Change code | Pull request | **Yes**, once ([CLA.md](CLA.md)) |

Found a vulnerability in Shieldbug itself? Do not open a public issue. Use GitHub's **Report a vulnerability** button (private).

## How checklist items move

| Tier | How an item gets there | Who receives it |
|---|---|---|
| New | Anyone files a proposal with a source link | Users who turned on "new" |
| Community | Enough weighted 👍, few 👎 | Users who turned on "community" |
| Default | Good real-use record, maintainer review, safety check | Every user (signed release) |

- **Going down is automatic. Going up needs a person.** Enough 👎 or false-alarm reports drop an item a tier or hide it. Votes alone never put an item into the default list.
- Votes from people who actually ran the item count most. New accounts count least. Sudden bursts of votes are put on hold.

## Rules for checklist items

1. **A public source is required:** an advisory, a CVE, a vendor post or a news report.
2. **The check must use facts Shieldbug already collects.** If it needs new data, say so. That is a code change.
3. **Fixes describe what to change, not commands to paste.** A fix with a shell command can never reach the default tier unless the command comes from the vendor's own documentation, linked, and the dashboard shows a warning next to it.
4. No working exploit code, malware samples, victims' personal data, or accusations without a public source.

## Rules for code

- Go standard library only. A new dependency needs a very good reason.
- Shieldbug **never fixes anything by itself** and stays read-only on the user's machine.
- Never add flags that skip an AI tool's permission checks (`--dangerously-skip-permissions`, `bypassPermissions`, `--yolo`, `danger-full-access` and the like).
- The analysis call gets **no tools**. Machine data goes through `redact()` and the gate before it leaves the machine.
- `go vet ./...` and `go test ./...` must pass. Add a test for redaction changes.
- Sign the [CLA](CLA.md) once. The bot asks on your first pull request.

---

## 한국어

**제안은 누구나, 반영은 검토를 거친 것만.** Shieldbug는 남의 PC에서 돌기 때문에 잘못된 변경은 곧 그 사람들에 대한 공격이 됩니다.

| 참여 방법 | 방법 | CLA 필요? |
|---|---|---|
| 사례 공유 (당한 사례, 막은 사례) | 이슈 양식 "사례" | 아니요 (CC BY 4.0) |
| 점검 항목 제안 | 이슈 양식 "점검 항목 제안", 출처 링크 필수 | 아니요 (CC BY 4.0) |
| 점검 항목 투표 | 제안 이슈에 👍/👎, 또는 대시보드의 "도움됨/오탐" (켠 사람만) | 아니요 |
| 코드 수정 | Pull Request | **예**, 처음 한 번 ([CLA.md](CLA.md)) |

- 점검 항목은 **새 제안 → 커뮤니티 검증 중 → 기본 점검** 순서로 올라갑니다. **내려가는 건 투표만으로 자동, 올라가는 건 사람 확인까지** 거쳐야 합니다.
- 해결법에는 "무엇을 바꾸라"를 쓰고, 붙여넣을 명령어는 쓰지 마세요. 명령어가 든 해결법은 공식 문서 출처와 경고 표시가 없으면 기본 점검으로 올라가지 못합니다.
- 공격 코드, 악성코드 샘플, 피해자 개인정보, 출처 없는 지목은 금지입니다.
- Shieldbug 자체의 취약점은 공개 이슈 말고 GitHub의 **Report a vulnerability**(비공개)로 알려 주세요.
