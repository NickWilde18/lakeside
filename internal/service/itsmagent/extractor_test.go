package itsmagent

import (
	"strings"
	"testing"
)

func TestNormalizeServiceLevel(t *testing.T) {
	cases := []struct {
		in   string
		out  string
		good bool
	}{
		{"1", "1", true},
		{"high", "2", true},
		{"中等", "3", true},
		{"low", "4", true},
		{"unknown", "", false},
	}

	for _, tc := range cases {
		got, ok := normalizeServiceLevel(tc.in)
		if got != tc.out || ok != tc.good {
			t.Fatalf("normalizeServiceLevel(%q) got=(%q,%v), want=(%q,%v)", tc.in, got, ok, tc.out, tc.good)
		}
	}
}

func TestNormalizePriority(t *testing.T) {
	cases := []struct {
		in   string
		out  string
		good bool
	}{
		{"咨询", "1", true},
		{"service", "2", true},
		{"故障", "3", true},
		{"feedback", "4", true},
		{"none", "", false},
	}

	for _, tc := range cases {
		got, ok := normalizePriority(tc.in)
		if got != tc.out || ok != tc.good {
			t.Fatalf("normalizePriority(%q) got=(%q,%v), want=(%q,%v)", tc.in, got, ok, tc.out, tc.good)
		}
	}
}

func TestNeedInfoInterruptDoesNotAskUserForServiceLevelDirectly(t *testing.T) {
	agent := NewTicketCreateAgent(nil, nil, nil, nil, nil, serviceConfig{EnumConfidenceThreshold: 0.75})
	info, incomplete := agent.needInfoInterrupt("zh", TicketDraft{
		UserCode:               "122020255",
		Subject:                "寝室WiFi故障",
		ServiceLevel:           "3",
		ServiceLevelConfidence: 0.4,
		Priority:               "3",
		PriorityConfidence:     1,
		OthersDesc:             "WiFi无法连接",
	}, "请提供寝室具体位置（楼号、房间号）及故障现象")
	if !incomplete {
		t.Fatalf("expected incomplete draft")
	}
	if len(info.MissingFields) != 1 || info.MissingFields[0] != "othersDesc" {
		t.Fatalf("unexpected missing fields: %#v", info.MissingFields)
	}
	if strings.Contains(info.Prompt, "服务级别") {
		t.Fatalf("prompt should not ask user to fill service level directly, got %q", info.Prompt)
	}
	if !strings.Contains(info.Prompt, "补充说明") {
		t.Fatalf("prompt should preserve clarify text as supplement, got %q", info.Prompt)
	}
}

func TestNeedInfoInterruptUsesEnglishPromptForEnglishUsers(t *testing.T) {
	agent := NewTicketCreateAgent(nil, nil, nil, nil, nil, serviceConfig{EnumConfidenceThreshold: 0.75})
	info, incomplete := agent.needInfoInterrupt("en", TicketDraft{
		UserCode:               "122020255",
		Subject:                "Dorm WiFi issue",
		ServiceLevel:           "3",
		ServiceLevelConfidence: 0.4,
		Priority:               "3",
		PriorityConfidence:     1,
		OthersDesc:             "Connected but no internet",
	}, "Please provide the building and room number.")
	if !incomplete {
		t.Fatalf("expected incomplete draft")
	}
	if strings.Contains(strings.ToLower(info.Prompt), "service level") {
		t.Fatalf("prompt should not ask English users to fill service level directly, got %q", info.Prompt)
	}
	if !strings.Contains(info.Prompt, "Additional details") {
		t.Fatalf("prompt should preserve clarify text in English, got %q", info.Prompt)
	}
}

func TestNeedInfoInterruptUsesVPNSpecificClarifyPrompt(t *testing.T) {
	agent := NewTicketCreateAgent(nil, nil, nil, nil, nil, serviceConfig{EnumConfidenceThreshold: 0.75})
	info, incomplete := agent.needInfoInterrupt("zh", TicketDraft{
		UserCode:               "122020255",
		Subject:                "VPN 无法连接",
		ServiceLevel:           "3",
		ServiceLevelConfidence: 0.4,
		Priority:               "3",
		PriorityConfidence:     1,
		OthersDesc:             "VPN 连不上，客户端报错。",
	}, "请问您是在哪里尝试连接VPN（具体楼号和房间号）？另外，请问连接失败的具体现象是什么？")
	if !incomplete {
		t.Fatalf("expected incomplete draft")
	}
	if strings.Contains(info.Prompt, "楼号") || strings.Contains(info.Prompt, "房间号") {
		t.Fatalf("vpn prompt should not ask for building or room, got %q", info.Prompt)
	}
	if !strings.Contains(info.Prompt, "登录页面") {
		t.Fatalf("vpn prompt should ask about login page / client error / campus resource access, got %q", info.Prompt)
	}
	if !strings.Contains(info.Prompt, "其他设备") {
		t.Fatalf("vpn prompt should ask about impact scope, got %q", info.Prompt)
	}
}

func TestDetectUserLanguage(t *testing.T) {
	if got := detectUserLanguage("宿舍 WiFi 坏了"); got != "zh" {
		t.Fatalf("detectUserLanguage chinese got %q", got)
	}
	if got := detectUserLanguage("Dorm WiFi is down"); got != "en" {
		t.Fatalf("detectUserLanguage english got %q", got)
	}
}

func TestBuildExtractPromptContainsServiceLevelAndPriorityRules(t *testing.T) {
	prompt := buildExtractPrompt(TicketDraft{}, "宿舍WiFi坏了", "无")
	if !strings.Contains(prompt, "默认填写 3（中）") {
		t.Fatalf("prompt should contain default serviceLevel rule, got %q", prompt)
	}
	if !strings.Contains(prompt, "多个人 / 多个寝室 / 多位同事 / 多台终端") {
		t.Fatalf("prompt should contain serviceLevel escalation rule, got %q", prompt)
	}
	if !strings.Contains(prompt, "WiFi 坏了、网页打不开、连不上网") {
		t.Fatalf("prompt should contain priority fault examples, got %q", prompt)
	}
	if !strings.Contains(prompt, "跨用户集中爆发的升级由系统在提交前根据近期相似工单聚合结果处理") {
		t.Fatalf("prompt should mention server-side escalation, got %q", prompt)
	}
	if !strings.Contains(prompt, "对 VPN 问题，完整信息优先覆盖") {
		t.Fatalf("prompt should contain vpn-specific guidance, got %q", prompt)
	}
	if !strings.Contains(prompt, "不要默认追问楼号和房间号") {
		t.Fatalf("prompt should forbid location-first vpn clarify question, got %q", prompt)
	}
}
