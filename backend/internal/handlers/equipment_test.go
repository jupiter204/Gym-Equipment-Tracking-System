package handlers

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildTrendMonths(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		loc = time.UTC
	}

	tests := []struct {
		name         string
		now          time.Time
		expectedKeys []string
		expectedName []string
	}{
		{
			name:         "2026-10-04",
			now:          time.Date(2026, 10, 4, 12, 0, 0, 0, loc),
			expectedKeys: []string{"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10"},
			expectedName: []string{"5月", "6月", "7月", "8月", "9月", "10月"},
		},
		{
			name:         "2026-10-31 (month end overflow test)",
			now:          time.Date(2026, 10, 31, 23, 59, 59, 0, loc),
			expectedKeys: []string{"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10"},
			expectedName: []string{"5月", "6月", "7月", "8月", "9月", "10月"},
		},
		{
			name:         "2026-08-31 (31-day month overflow test)",
			now:          time.Date(2026, 8, 31, 10, 0, 0, 0, loc),
			expectedKeys: []string{"2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08"},
			expectedName: []string{"3月", "4月", "5月", "6月", "7月", "8月"},
		},
		{
			name:         "2026-03-30 (leap/february boundary test)",
			now:          time.Date(2026, 3, 30, 8, 0, 0, 0, loc),
			expectedKeys: []string{"2025-10", "2025-11", "2025-12", "2026-01", "2026-02", "2026-03"},
			expectedName: []string{"10月", "11月", "12月", "1月", "2月", "3月"},
		},
		{
			name:         "2026-05-31",
			now:          time.Date(2026, 5, 31, 15, 30, 0, 0, loc),
			expectedKeys: []string{"2025-12", "2026-01", "2026-02", "2026-03", "2026-04", "2026-05"},
			expectedName: []string{"12月", "1月", "2月", "3月", "4月", "5月"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := buildTrendMonths(tc.now)
			if len(res) != 6 {
				t.Fatalf("expected 6 months, got %d", len(res))
			}
			keys := make([]string, 6)
			names := make([]string, 6)
			for i, item := range res {
				keys[i] = item.MonthKey
				names[i] = item.Name
			}
			if !reflect.DeepEqual(keys, tc.expectedKeys) {
				t.Errorf("MonthKey mismatch: got %v, want %v", keys, tc.expectedKeys)
			}
			if !reflect.DeepEqual(names, tc.expectedName) {
				t.Errorf("Name mismatch: got %v, want %v", names, tc.expectedName)
			}
		})
	}
}
