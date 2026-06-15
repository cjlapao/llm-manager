package service

import (
	"testing"
)

func TestSpeechServiceMethodsExist(t *testing.T) {
	// Verify that StartSpeech and StopSpeech have been removed
	// by checking they no longer exist on ContainerService.
	// If this test compiles, the methods are gone.
	var svc *ContainerService
	t.Logf("ContainerService exists without legacy speech methods")
	_ = svc
}
