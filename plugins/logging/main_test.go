package logging

import (
	"encoding/json"
	"testing"

	"github.com/pin-gou/pg-gateway/framework/logstore"
)

func TestConfig_UnmarshalJSON_Defaults(t *testing.T) {
	// Config has no custom UnmarshalJSON — all pointer fields are nil when absent.
	var cfg Config
	data := `{}`
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if cfg.DisableContentLogging != nil {
		t.Errorf("DisableContentLogging: expected nil, got %v", *cfg.DisableContentLogging)
	}
	if cfg.RetainContentInObjectStorage != nil {
		t.Errorf("RetainContentInObjectStorage: expected nil, got %v", *cfg.RetainContentInObjectStorage)
	}
	if cfg.LoggingHeaders != nil {
		t.Errorf("LoggingHeaders: expected nil, got non-nil")
	}
	if cfg.Writer != nil {
		t.Errorf("Writer: expected nil, got non-nil")
	}
}

func TestConfig_UnmarshalJSON_WithValues(t *testing.T) {
	data := `{
		"disable_content_logging": true,
		"retain_content_in_object_storage": true,
		"logging_headers": ["x-custom"],
		"writer": {
			"max_batch_size": 500,
			"batch_interval": "2s",
			"max_batch_bytes": 1048576,
			"write_queue_capacity": 5000,
			"deferred_usage_concurrency": 3
		}
	}`
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if cfg.DisableContentLogging == nil || *cfg.DisableContentLogging != true {
		t.Errorf("DisableContentLogging: expected true, got %v", cfg.DisableContentLogging)
	}
	if cfg.RetainContentInObjectStorage == nil || *cfg.RetainContentInObjectStorage != true {
		t.Errorf("RetainContentInObjectStorage: expected true, got %v", cfg.RetainContentInObjectStorage)
	}
	if cfg.LoggingHeaders == nil || len(*cfg.LoggingHeaders) != 1 || (*cfg.LoggingHeaders)[0] != "x-custom" {
		t.Errorf("LoggingHeaders: expected [x-custom], got %v", cfg.LoggingHeaders)
	}
	if cfg.Writer == nil {
		t.Fatal("Writer: expected non-nil")
	}
	if cfg.Writer.MaxBatchSize != 500 {
		t.Errorf("MaxBatchSize: expected 500, got %d", cfg.Writer.MaxBatchSize)
	}
	if cfg.Writer.BatchInterval != "2s" {
		t.Errorf("BatchInterval: expected 2s, got %s", cfg.Writer.BatchInterval)
	}
}

func TestValidateWriterConfig_MaxBatchSize_Zero(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             0,
		BatchInterval:            "5s",
		MaxBatchBytes:            1024,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err == nil {
		t.Error("expected error for MaxBatchSize=0")
	}
}

func TestValidateWriterConfig_MaxBatchSize_Negative(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             -1,
		BatchInterval:            "5s",
		MaxBatchBytes:            1024,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err == nil {
		t.Error("expected error for MaxBatchSize=-1")
	}
}

func TestValidateWriterConfig_BatchInterval_Empty(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             100,
		BatchInterval:            "",
		MaxBatchBytes:            1024,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err == nil {
		t.Error("expected error for empty BatchInterval")
	}
}

func TestValidateWriterConfig_BatchInterval_InvalidDuration(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             100,
		BatchInterval:            "not-a-duration",
		MaxBatchBytes:            1024,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err == nil {
		t.Error("expected error for invalid BatchInterval")
	}
}

func TestValidateWriterConfig_BatchInterval_ZeroDuration(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             100,
		BatchInterval:            "0s",
		MaxBatchBytes:            1024,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err == nil {
		t.Error("expected error for BatchInterval=0s")
	}
}

func TestValidateWriterConfig_MaxBatchBytes_Zero(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             100,
		BatchInterval:            "5s",
		MaxBatchBytes:            0,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err == nil {
		t.Error("expected error for MaxBatchBytes=0")
	}
}

func TestValidateWriterConfig_MaxBatchBytes_Negative(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             100,
		BatchInterval:            "5s",
		MaxBatchBytes:            -1,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err == nil {
		t.Error("expected error for MaxBatchBytes=-1")
	}
}

func TestValidateWriterConfig_Valid(t *testing.T) {
	err := validateWriterConfig(logstore.WriterConfig{
		MaxBatchSize:             100,
		BatchInterval:            "5s",
		MaxBatchBytes:            1024,
		WriteQueueCapacity:       100,
		DeferredUsageConcurrency: 5,
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}