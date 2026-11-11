package main

import "fmt"

var dict = map[string][2]string{ // en, ko
	"tab_today":        {"Today", "오늘"},
	"tab_findings":     {"Findings", "발견"},
	"tab_checklist":    {"Checklist", "점검 목록"},
	"tab_history":      {"History", "기록"},
	"tab_settings":     {"Settings", "설정"},
	"tab_sent":         {"What was sent", "보낸 내용"},
	"run_now":          {"Check now", "지금 점검하기"},
	"running":          {"Checking…", "점검하는 중이에요…"},
	"p_selftest":       {"Testing the redaction", "가림 처리를 시험하고 있어요"},
	"p_collect":        {"Reading local settings", "내 PC 설정을 읽고 있어요"},
	"p_intel":          {"Looking up today's threats", "오늘의 새 위협을 찾고 있어요"},
	"p_analysis":       {"AI is reviewing", "AI가 검토하고 있어요"},
	"no_runs":          {"No check yet. Shall we run the first one?", "아직 점검한 적이 없어요. 첫 점검을 시작해 볼까요?"},
	"score":            {"Score", "점수"},
	"score_pts":        {"%d points", "%d점"},
	"light_green":      {"You're in good shape", "안전해요"},
	"light_yellow":     {"Needs attention", "주의가 필요해요"},
	"light_red":        {"Act now", "지금 조치가 필요해요"},
	"light_none":       {"Ready for a first check", "첫 점검을 기다리고 있어요"},
	"last_check":       {"Last check", "마지막 점검"},
	"todo":             {"Do these first", "먼저 해볼 일"},
	"to_fix":           {"%d to fix", "고칠 것 %d개"},
	"no_findings":      {"Nothing to fix. Shieldbug only checks what is on its checklist; it does not replace your antivirus.", "고칠 게 없어요. Shieldbug는 점검 목록에 있는 것만 봐요. 백신을 대신하진 않아요."},
	"intel":            {"Today's new threats", "오늘의 새 위협"},
	"no_intel":         {"No threat search in this run.", "이번 점검에선 위협 검색을 하지 않았어요."},
	"this_run":         {"This run", "이번 점검"},
	"agent":            {"AI", "AI"},
	"model":            {"Model", "모델"},
	"tokens":           {"Tokens in / out", "토큰 (입력 / 출력)"},
	"cost":             {"Cost (estimate)", "예상 비용"},
	"cost_note":        {"Reported by the CLI. On a subscription it comes out of your plan's usage, not extra money.", "CLI가 알려 준 추정값이에요. 구독 중이면 추가 결제 없이 구독 사용량에서 빠져요."},
	"duration":         {"Time", "걸린 시간"},
	"lockdown":         {"Tool lockdown", "도구 차단"},
	"lock_full":        {"all tools off", "도구 전부 꺼짐"},
	"key_set":          {"API key set", "API 키 있음"},
	"no_key":           {"no API key", "API 키 없음"},
	"lock_read-only":   {"read-only sandbox", "읽기 전용 샌드박스"},
	"lock_plan-mode":   {"plan mode", "계획 모드"},
	"lock_unverified":  {"not verified", "확인 안 됨"},
	"evidence":         {"Evidence", "근거"},
	"fix":              {"How to fix", "이렇게 고치세요"},
	"copy":             {"Copy", "복사"},
	"copied":           {"Copied", "복사했어요"},
	"source_local":     {"built-in rule", "기본 규칙"},
	"source_ai":        {"AI review", "AI 판단"},
	"sev_critical":     {"Critical", "매우 위험"},
	"sev_high":         {"High", "위험"},
	"sev_medium":       {"Medium", "주의"},
	"sev_low":          {"Low", "참고"},
	"pending":          {"AI suggested new checks", "AI가 새 점검 항목을 제안했어요"},
	"pending_sub":      {"Add the ones that make sense for you. They only change your own checklist.", "필요한 것만 추가하세요. 내 점검 목록에만 반영돼요."},
	"approve":          {"Add", "추가하기"},
	"reject":           {"No thanks", "괜찮아요"},
	"active_items":     {"Checks in use", "지금 쓰는 점검 항목"},
	"history_trend":    {"Score over time", "점수 변화"},
	"when":             {"When", "시각"},
	"findings_n":       {"Findings", "발견"},
	"errors":           {"Notes", "메모"},
	"scheduled":        {"scheduled", "자동"},
	"manual":           {"manual", "직접"},
	"save":             {"Save", "저장하기"},
	"daily_at":         {"Daily check time", "매일 점검 시각"},
	"schedule":         {"Daily automatic check", "매일 자동 점검"},
	"sched_on":         {"On", "켜져 있어요"},
	"sched_off":        {"Off", "꺼져 있어요"},
	"sched_on_sub":     {"Registered with your OS scheduler. Nothing stays running in the background.", "운영체제 예약 작업에 등록돼 있어요. 평소엔 아무것도 떠 있지 않아요."},
	"sched_off_sub":    {"Turn it on and Shieldbug checks once a day by itself.", "켜 두면 하루 한 번 알아서 점검해요."},
	"sched_install":    {"Turn on", "켜기"},
	"sched_remove":     {"Turn off", "끄기"},
	"model_search":     {"Also search the web for today's threats (a separate call that never sees your machine's data)", "오늘의 새 위협도 웹에서 찾기 (내 PC 정보는 보지 않는 별도 호출이에요)"},
	"language":         {"Language", "언어"},
	"effort":           {"Reasoning effort", "추론 강도"},
	"auto":             {"Auto", "자동"},
	"detected":         {"installed", "설치됨"},
	"not_found":        {"not found", "없음"},
	"sent_intro":       {"This is exactly what the analysis call received. Secrets were replaced before sending.", "AI 분석에 보낸 내용 그대로예요. 비밀 값은 보내기 전에 가렸어요."},
	"no_payload":       {"Nothing stored for this run.", "이 점검에서 저장된 내용이 없어요."},
	"saved":            {"Saved.", "저장했어요."},
	"scope":            {"Checks AI agent settings, secrets hygiene, open ports and OS protection on this computer. Not an antivirus.", "AI 에이전트 설정, 비밀 값 관리, 열린 포트, OS 보호 상태를 점검해요. 백신은 아니에요."},
	"just_now":         {"just now", "방금"},
	"min_ago":          {"%d min ago", "%d분 전"},
	"h_ago":            {"%d h ago", "%d시간 전"},
	"d_ago":            {"%d days ago", "%d일 전"},
	"proposals_banner": {"AI suggested %d new checks", "AI가 새 점검 항목 %d개를 제안했어요"},
	"open_list":        {"Review", "검토하기"},
	"community":        {"Share a case", "사례 공유"},
	"status":           {"Status", "상태"},
	"agent_pick":       {"AI to use", "사용할 AI"},
	"agent_hint":       {"Only the AIs you tick are used. Tick several and the top one goes first; the next takes over only if a call fails. Tick none and only the built-in checks run.", "체크한 AI만 써요. 여러 개 고르면 위에서부터 쓰고, 막히면 다음 AI로 넘어가요. 하나도 안 고르면 AI 없이 기본 점검만 해요."},
	"no_major":         {"Nothing urgent. Shieldbug only checks what is on its checklist; it does not replace your antivirus.", "급하게 고칠 건 없어요. Shieldbug는 점검 목록에 있는 것만 봐요. 백신을 대신하진 않아요."},
	"minor_more":       {"%d more worth a look, not urgent", "급하진 않지만 참고할 것 %d개"},
	"minor_title":      {"Worth a look (%d)", "참고할 것 (%d)"},
	"minor_hint":       {"Real but not urgent: each needs something else to go wrong first. Fix them when you have time.", "실제 약점이지만 다른 조건이 하나 더 있어야 문제가 돼요. 시간 날 때 고치면 돼요."},
	"used_first":       {"Used first", "먼저 써요"},
	"models_effort":    {"Model for each AI", "AI별 모델"},
	"general":          {"Schedule and language", "일정과 언어"},
	"view_sent":        {"See what was sent to the AI", "AI에게 보낸 내용 보기"},
	"all_findings":     {"See all", "전체 보기"},
	"show_evidence":    {"Show evidence", "근거 보기"},
	"from_ai":          {"added by AI", "AI가 추가"},
	"no_history":       {"The trend appears after two or more checks.", "두 번 이상 점검하면 변화가 보여요."},
	"cost_short":       {"Cost", "비용"},
	"tokens_short":     {"Tokens", "토큰"},
	"time_short":       {"Time", "걸린 시간"},
	"findings_sub":     {"%d found · %s", "%d개 발견 · %s"},
	"history_sub":      {"Your last %d checks", "최근 점검 %d번"},
	"checklist_sub":    {"%d checks in use. AI suggests new ones when new attacks appear.", "점검 항목 %d개를 쓰고 있어요. 새 공격이 나오면 AI가 항목을 제안해요."},
	"plan_5h":          {"5-hour limit ($20 plan)", "5시간 한도 (20달러 구독 기준)"},
	"plan_week":        {"Weekly limit ($20 plan)", "주간 한도 (20달러 구독 기준)"},
	"plan_note":        {"Share of a $20 plan (Claude Pro, ChatGPT Plus) one check uses, estimated from measured usage.", "한 번 점검이 20달러 구독(Claude Pro, ChatGPT Plus) 한도에서 차지하는 몫이에요. 실측한 사용량으로 추정했어요."},
	"pct_about":        {"about %s", "약 %s"},
	"pct_tiny":         {"under 0.1%", "0.1% 미만"},
	"reload":           {"Reload (picks up a new version and changes to config.json or the checklist)", "새로고침 (새 버전, config.json·점검 목록을 바꾼 내용이 반영돼요)"},
	"updating":         {"Switching to the new version…", "새 버전으로 바꾸는 중이에요…"},
	"theme_toggle":     {"Switch light or dark mode", "라이트·다크 모드 바꾸기"},
	"steps_title":      {"How your AI checks", "AI가 이렇게 점검해요"},
	"steps_done":       {"All 4 steps done", "4단계 모두 마쳤어요"},
	"st_selftest":      {"Hide secrets", "비밀 값 가리기"},
	"st_collect":       {"Read PC settings", "내 PC 설정 읽기"},
	"st_intel":         {"Find new threats", "새 위협 찾기"},
	"st_analysis":      {"AI review", "AI 검토"},
	"stat_fix":         {"To fix", "고칠 것"},
	"stat_checks":      {"Checks", "점검 항목"},
	"stat_intel":       {"New threats", "새 위협"},
	"unit_items":       {"%d", "%d개"},
	"unit_cases":       {"%d", "%d건"},
	"hero_sub":         {"%d points · checked %s", "%d점 · %s 점검"},
	"hello_none":       {"Your AI looks after this PC's security, one checklist at a time.", "내 AI가 점검 목록대로 이 PC의 보안을 챙겨 드려요."},
	"settings_sub":     {"Everything here can also be changed by your own AI in ~/.shieldbug/config.json.", "여기 있는 건 모두 내 AI에게 시켜서 ~/.shieldbug/config.json으로 바꿀 수도 있어요."},
}

// agentNames: what people see for each agent id. The id stays in config.json.
var agentNames = map[string][2]string{
	"claude":     {"Claude (Claude Code)", "Claude (Claude Code)"},
	"codex":      {"GPT (Codex)", "GPT (Codex)"},
	"agy":        {"Gemini (Antigravity)", "Gemini (Antigravity)"},
	"claude-api": {"Claude (API key)", "Claude (API 키)"},
	"openai-api": {"GPT (API key)", "GPT (API 키)"},
	"gemini-api": {"Gemini (API key)", "Gemini (API 키)"},
}

func agentName(lang, id string) string {
	v, ok := agentNames[id]
	if !ok {
		return id
	}
	if lang == "ko" {
		return v[1]
	}
	return v[0]
}

func tr(lang, key string) string {
	v, ok := dict[key]
	if !ok {
		return key
	}
	if lang == "ko" {
		return v[1]
	}
	return v[0]
}

func trf(lang, key string, a ...any) string { return fmt.Sprintf(tr(lang, key), a...) }
