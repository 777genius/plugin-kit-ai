package usecase

import (
	"errors"
	"testing"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func TestBatchUpdateSkipWarningPreservesMessageContract(t *testing.T) {
	t.Parallel()

	if got := batchUpdateSkipWarning("demo", errors.New("boom")); got != "Update skipped for demo: boom" {
		t.Fatalf("warning = %q", got)
	}
}

func TestLessSyncTargetReportOrdersByTargetThenDelivery(t *testing.T) {
	t.Parallel()

	if !lessSyncTargetReport(domain.TargetReport{TargetID: "claude", DeliveryKind: "b"}, domain.TargetReport{TargetID: "gemini", DeliveryKind: "a"}) {
		t.Fatal("expected claude before gemini")
	}
	if !lessSyncTargetReport(domain.TargetReport{TargetID: "gemini", DeliveryKind: "a"}, domain.TargetReport{TargetID: "gemini", DeliveryKind: "b"}) {
		t.Fatal("expected delivery-kind ordering for same target")
	}
}
