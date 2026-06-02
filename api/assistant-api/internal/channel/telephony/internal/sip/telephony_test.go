package internal_sip_telephony

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rapidaai/api/assistant-api/config"
	internal_sip "github.com/rapidaai/api/assistant-api/internal/channel/telephony/internal/sip/internal"
	internal_assistant_entity "github.com/rapidaai/api/assistant-api/internal/entity/assistants"
	sip_infra "github.com/rapidaai/api/assistant-api/sip/infra"
	"github.com/rapidaai/pkg/commons"
	"github.com/rapidaai/protos"
	"google.golang.org/protobuf/types/known/structpb"
)

func vaultCredential(t *testing.T, values map[string]interface{}) *protos.VaultCredential {
	t.Helper()
	v, err := structpb.NewStruct(values)
	if err != nil {
		t.Fatalf("failed to create vault credential: %v", err)
	}
	return &protos.VaultCredential{Value: v}
}

func newSIPTelephonyForTest() *sipTelephony {
	logger, _ := commons.NewApplicationLogger()
	return &sipTelephony{
		logger: logger,
		appCfg: &config.AssistantConfig{
			SIPConfig: &config.SIPConfig{
				Port:              5060,
				Transport:         "udp",
				RTPPortRangeStart: 10000,
				RTPPortRangeEnd:   10100,
				RegisterTimeout:   5 * time.Second,
				InviteTimeout:     30 * time.Second,
				SessionTimeout:    45 * time.Minute,
			},
		},
	}
}

func TestParseConfig_UsesPortFromSIPURI(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"sip_uri":      "sip:example.org:5097",
		"sip_username": "user",
		"sip_password": "pass",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.Port != 5097 {
		t.Fatalf("expected parsed SIP URI port 5097, got %d", cfg.Port)
	}
}

func TestParseConfig_UsesExplicitSIPPortFromVault(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"sip_server":   "example.org",
		"sip_port":     5098,
		"sip_username": "user",
		"sip_password": "pass",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.Port != 5098 {
		t.Fatalf("expected explicit vault sip_port 5098, got %d", cfg.Port)
	}
}

func TestParseConfig_DefaultsOutboundTo5060WhenVaultPortMissing(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"sip_server":   "example.org",
		"sip_username": "user",
		"sip_password": "pass",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.Port != internal_sip.DefaultOutboundSIPPort {
		t.Fatalf("expected default outbound SIP port %d, got %d", internal_sip.DefaultOutboundSIPPort, cfg.Port)
	}
}

func TestParseConfig_AllowsOutboundWithoutAuth(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"host": "example.org:5060",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.Server != "example.org" {
		t.Fatalf("expected server example.org, got %q", cfg.Server)
	}
	if cfg.Username != "" || cfg.Password != "" {
		t.Fatalf("expected empty auth, got username=%q password=%q", cfg.Username, cfg.Password)
	}
}

func TestParseConfig_AppliesPlatformTimeouts(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"host": "example.org:5060",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.RegisterTimeout != 5*time.Second {
		t.Fatalf("expected register timeout 5s, got %s", cfg.RegisterTimeout)
	}
	if cfg.InviteTimeout != 30*time.Second {
		t.Fatalf("expected invite timeout 30s, got %s", cfg.InviteTimeout)
	}
	if cfg.SessionTimeout != 45*time.Minute {
		t.Fatalf("expected session timeout 45m, got %s", cfg.SessionTimeout)
	}
}

func TestParseConfig_AppliesInboundAnswerPolicyDefaults(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	telephony.appCfg.SIPConfig.Inbound = config.SIPInboundConfig{
		AnswerMode:                 string(sip_infra.InboundAnswerModeAfterMinRingDuration),
		MinRingDuration:            50 * time.Millisecond,
		MaxRingDuration:            5 * time.Second,
		ACKTimeout:                 2 * time.Second,
		AssistantAudioReadyTimeout: 250 * time.Millisecond,
		RequireAssistantAudioReady: true,
	}
	cred := vaultCredential(t, map[string]interface{}{
		"host": "example.org:5060",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.InboundAnswerMode != sip_infra.InboundAnswerModeAfterMinRingDuration {
		t.Fatalf("expected inbound answer mode from app config, got %q", cfg.InboundAnswerMode)
	}
	if cfg.InboundMinRingDuration != 50*time.Millisecond ||
		cfg.InboundMaxRingDuration != 5*time.Second ||
		cfg.InboundACKTimeout != 2*time.Second ||
		cfg.InboundAssistantAudioReadyTimeout != 250*time.Millisecond ||
		!cfg.InboundRequireAssistantAudioReady {
		t.Fatalf("expected inbound answer policy defaults from app config, got %#v", cfg)
	}
}

func TestOutboundHealthGateEnabled_DefaultsOn(t *testing.T) {
	if !outboundHealthGateEnabled(nil) {
		t.Fatal("expected health gate enabled for nil app config")
	}
	if !outboundHealthGateEnabled(&config.AssistantConfig{}) {
		t.Fatal("expected health gate enabled without SIP config")
	}
}

func TestOutboundHealthGateEnabled_AllowsExplicitDisable(t *testing.T) {
	disabled := false
	appCfg := &config.AssistantConfig{
		SIPConfig: &config.SIPConfig{OutboundHealthGate: &disabled},
	}
	if outboundHealthGateEnabled(appCfg) {
		t.Fatal("expected health gate disabled")
	}
}

func TestParseConfig_ParsesCustomHeaders(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"sip_uri":      "sip:example.org:5060",
		"sip_username": "user",
		"sip_password": "pass",
		"sip_headers":  `{"X-Piopiy-Username":"Nitin","X-Custom":"value"}`,
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if len(cfg.CustomHeaders) != 2 {
		t.Fatalf("expected 2 custom headers, got %d", len(cfg.CustomHeaders))
	}
	if cfg.CustomHeaders["X-Piopiy-Username"] != "Nitin" {
		t.Fatalf("expected X-Piopiy-Username=Nitin, got %s", cfg.CustomHeaders["X-Piopiy-Username"])
	}
	if cfg.CustomHeaders["X-Custom"] != "value" {
		t.Fatalf("expected X-Custom=value, got %s", cfg.CustomHeaders["X-Custom"])
	}
}

func TestParseConfig_NoCustomHeadersWhenMissing(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"sip_uri":      "sip:example.org:5060",
		"sip_username": "user",
		"sip_password": "pass",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.CustomHeaders != nil {
		t.Fatalf("expected nil custom headers, got %v", cfg.CustomHeaders)
	}
}

func TestNewOutboundInitiatedCallInfo_UsesInitiatedStatus(t *testing.T) {
	session, err := sip_infra.NewSession(context.Background(), &sip_infra.SessionConfig{
		Config: &sip_infra.Config{
			Server:            "trunk.example.org",
			Port:              5060,
			Transport:         sip_infra.TransportUDP,
			RTPPortRangeStart: 10000,
			RTPPortRangeEnd:   10100,
		},
		Direction: sip_infra.CallDirectionOutbound,
		CallID:    "sip-call-1",
		Codec:     &sip_infra.CodecPCMU,
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	info := newOutboundInitiatedCallInfo(session, "+15551234567", "+15557654321", 42, 99)

	if info.Status != string(sip_infra.OutboundCallStatusInitiated) {
		t.Fatalf("expected initiated status, got %q", info.Status)
	}
	if info.Status == "SUCCESS" {
		t.Fatal("outbound call must not report SUCCESS before answer")
	}
	if info.StatusInfo.Event != string(sip_infra.OutboundCallStatusInitiated) {
		t.Fatalf("expected initiated event, got %q", info.StatusInfo.Event)
	}
	if info.Extra["telephony.status"] != string(sip_infra.OutboundCallStatusInitiated) {
		t.Fatalf("expected telephony.status initiated, got %q", info.Extra["telephony.status"])
	}
}

func TestOutboundAssistantID_AllowsNilAssistant(t *testing.T) {
	if got := outboundAssistantID(nil); got != 0 {
		t.Fatalf("expected nil assistant ID 0, got %d", got)
	}
	assistant := &internal_assistant_entity.Assistant{}
	assistant.Id = 42
	if got := outboundAssistantID(assistant); got != 42 {
		t.Fatalf("expected assistant ID 42, got %d", got)
	}
}

func TestParseConfig_InvalidJSONHeadersIgnored(t *testing.T) {
	telephony := newSIPTelephonyForTest()
	cred := vaultCredential(t, map[string]interface{}{
		"sip_uri":      "sip:example.org:5060",
		"sip_username": "user",
		"sip_password": "pass",
		"sip_headers":  "not-json",
	})

	cfg, err := telephony.parseConfig(cred)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}

	if cfg.CustomHeaders != nil {
		t.Fatalf("expected nil custom headers for invalid JSON, got %v", cfg.CustomHeaders)
	}
}

func TestReceiveCall_PopulatesDialedNumberFromFallbackParams(t *testing.T) {
	telephony := newSIPTelephonyForTest()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?caller=15551234567&destination=18005550100&call_id=sip-call-1", nil)
	c.Request = req

	info, err := telephony.ReceiveCall(c)
	if err != nil {
		t.Fatalf("ReceiveCall() error = %v", err)
	}

	if info.CallerNumber != "15551234567" {
		t.Fatalf("expected CallerNumber 15551234567, got %q", info.CallerNumber)
	}
	if info.FromNumber != "18005550100" {
		t.Fatalf("expected FromNumber from destination fallback, got %q", info.FromNumber)
	}
	if info.ChannelUUID != "sip-call-1" {
		t.Fatalf("expected ChannelUUID sip-call-1, got %q", info.ChannelUUID)
	}
	payload, ok := info.StatusInfo.Payload.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string payload, got %T", info.StatusInfo.Payload)
	}
	if got := payload["destination"]; got != "18005550100" {
		t.Fatalf("expected status payload destination=18005550100, got %q", got)
	}
}
