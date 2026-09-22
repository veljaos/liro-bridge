package main

import (
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// TestEveryStoreSaysSomethingDifferent is the point of the settings
// row: SPEC §6.4 has two branches on Linux and F12 §7 asks that a
// person be able to see which one they got. A row that said the same
// thing for both would satisfy every "is the row there" test and none
// of the reason the row exists.
func TestEveryStoreSaysSomethingDifferent(t *testing.T) {
	c := i18n.Load("en")
	cases := map[string]platform.SecretStoreDescription{
		"keyring": {Mechanism: platform.MechanismSecretService, Detail: "Login"},
		"file":    {Mechanism: platform.MechanismEncryptedFile, Fallback: true, Reason: "the keyring is locked"},
		"dpapi":   {Mechanism: platform.MechanismDPAPI},
		"none":    {},
	}
	seen := map[string]string{}
	for name, d := range cases {
		got := secretStoreSentence(c, d)
		if got == "" {
			t.Errorf("%s: the settings row is empty, so a person is told nothing", name)
			continue
		}
		if strings.HasPrefix(got, "settings.") {
			t.Errorf("%s: the sentence is the catalogue key %q, so a string is missing", name, got)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("%s and %s both say %q, so the row cannot tell them apart", name, other, got)
		}
		seen[got] = name
	}
}

// TestTheFallbackSaysWhy: a person who is told their keyring is not in
// use and not told why has been given a problem rather than
// information. SPEC §6.4 requires the warning; F12 §7 requires it be
// visible; neither is satisfied by "something went wrong".
func TestTheFallbackSaysWhy(t *testing.T) {
	c := i18n.Load("en")
	const reason = "this desktop has no Secret Service"
	got := secretStoreSentence(c, platform.SecretStoreDescription{
		Mechanism: platform.MechanismEncryptedFile, Fallback: true, Reason: reason,
	})
	if !strings.Contains(got, reason) {
		t.Errorf("the fallback row is %q and does not carry the reason %q", got, reason)
	}
}

// TestTheKeyringRowNamesTheCollection: "your keyring" is not checkable
// by the person reading it. The collection's own label is the thing
// they can go and open in Seahorse and find this program's items in.
func TestTheKeyringRowNamesTheCollection(t *testing.T) {
	c := i18n.Load("en")
	got := secretStoreSentence(c, platform.SecretStoreDescription{
		Mechanism: platform.MechanismSecretService, Detail: "Login",
	})
	if !strings.Contains(got, "Login") {
		t.Errorf("the keyring row is %q and does not name the collection", got)
	}
}

// TestTheSettingsPayloadCarriesTheStore checks the value actually
// reaches the page, rather than the sentence merely existing. The
// window renders from this map and from nothing else.
func TestTheSettingsPayloadCarriesTheStore(t *testing.T) {
	c := i18n.Load("en")
	init := buildSettingsInit(c, config.Default(), nil,
		platform.SecretStoreDescription{Mechanism: platform.MechanismEncryptedFile, Fallback: true, Reason: "no bus"})
	model, ok := init["model"].(map[string]any)
	if !ok {
		t.Fatal("the settings payload has no model")
	}
	text, _ := model["secretStoreText"].(string)
	if !strings.Contains(text, "no bus") {
		t.Errorf("the settings payload's secretStoreText is %q", text)
	}
	if fallback, _ := model["secretStoreFallback"].(bool); !fallback {
		t.Error("the settings payload does not mark the file branch as a fallback, " +
			"so the page renders it in the same ink as everything else")
	}
	if labels, ok := init["strings"].(map[string]string); !ok {
		t.Fatal("the settings payload has no strings")
	} else if labels["settings.secret_store_label"] == "" {
		t.Error("the settings payload carries no label for the secret-store row")
	}
}
