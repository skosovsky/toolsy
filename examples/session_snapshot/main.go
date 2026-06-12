// Package main demonstrates ExportSnapshot → JSON → ImportSnapshot roundtrip (toolsy v1.0).
package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/skosovsky/toolsy"
)

type agentPrefs struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
}

const (
	stateKey  = "prefs"
	demoCount = 3
)

func main() {
	reg := mustRegistry()
	sess := mustSession(reg)

	toolsy.SetSessionState(sess, stateKey, agentPrefs{Locale: "ru", Count: demoCount})

	snap, err := sess.ExportSnapshot()
	if err != nil {
		log.Fatal(err)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("snapshot bytes: %d\n", len(raw))

	restored, err := toolsy.NewSessionSnapshotFromJSON(raw)
	if err != nil {
		log.Fatal(err)
	}

	sess2 := mustSession(reg)
	if err := sess2.ImportSnapshot(restored); err != nil {
		log.Fatal(err)
	}

	got, ok := toolsy.GetSessionState[agentPrefs](sess2, stateKey)
	if !ok {
		log.Fatal("prefs missing after import")
	}
	fmt.Printf("restored prefs: locale=%s count=%d\n", got.Locale, got.Count)
}

func mustRegistry() *toolsy.Registry {
	reg, err := toolsy.NewRegistryBuilder().Build()
	if err != nil {
		log.Fatal(err)
	}
	return reg
}

func mustSession(reg *toolsy.Registry) *toolsy.Session {
	codecs := toolsy.NewStateCodecRegistry()
	if err := toolsy.RegisterJSONCodec[agentPrefs](codecs, stateKey); err != nil {
		log.Fatal(err)
	}
	sess, err := toolsy.NewSession(reg,
		toolsy.WithStateCodecRegistry(codecs),
		toolsy.WithStrictStateCodecs(true),
	)
	if err != nil {
		log.Fatal(err)
	}
	return sess
}
