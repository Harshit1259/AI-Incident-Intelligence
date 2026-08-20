package ratelimit_test

import (
	"testing"

	"ai-incident-platform/backend/internal/platform/ratelimit"
)

func TestAllow_WithinLimit(t *testing.T) {
	loader := func(_ string, _ ratelimit.Category) int { return 10 }
	l := ratelimit.New(loader)

	for i := 0; i < 10; i++ {
		if !l.Allow("tenant-1", ratelimit.CategoryQuery) {
			t.Fatalf("request %d should be allowed (limit 10)", i+1)
		}
	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	loader := func(_ string, _ ratelimit.Category) int { return 3 }
	l := ratelimit.New(loader)

	l.Allow("t", ratelimit.CategoryQuery)
	l.Allow("t", ratelimit.CategoryQuery)
	l.Allow("t", ratelimit.CategoryQuery)

	if l.Allow("t", ratelimit.CategoryQuery) {
		t.Error("4th request should be denied after exhausting limit of 3")
	}
}

func TestAllow_Unlimited(t *testing.T) {
	loader := func(_ string, _ ratelimit.Category) int { return 0 } // 0 = unlimited
	l := ratelimit.New(loader)

	for i := 0; i < 10000; i++ {
		if !l.Allow("tenant-unlimited", ratelimit.CategoryQuery) {
			t.Fatalf("unlimited tenant: request %d denied", i+1)
		}
	}
}

func TestAllow_PerTenantIsolation(t *testing.T) {
	loader := func(_ string, _ ratelimit.Category) int { return 1 }
	l := ratelimit.New(loader)

	// Exhaust tenant-A.
	l.Allow("tenant-a", ratelimit.CategoryQuery)

	if l.Allow("tenant-a", ratelimit.CategoryQuery) {
		t.Error("tenant-a: second request should be denied")
	}
	if !l.Allow("tenant-b", ratelimit.CategoryQuery) {
		t.Error("tenant-b: must not be affected by tenant-a's exhausted bucket")
	}
}

func TestAllow_PerCategoryIsolation(t *testing.T) {
	loader := func(_ string, cat ratelimit.Category) int {
		if cat == ratelimit.CategoryMutation {
			return 1
		}
		return 1000
	}
	l := ratelimit.New(loader)

	// Exhaust mutation.
	l.Allow("t", ratelimit.CategoryMutation)

	if l.Allow("t", ratelimit.CategoryMutation) {
		t.Error("mutation: second request should be denied")
	}
	if !l.Allow("t", ratelimit.CategoryQuery) {
		t.Error("query: must not be affected by mutation bucket being exhausted")
	}
}

func TestEvict_ClearsBuckets(t *testing.T) {
	loader := func(_ string, _ ratelimit.Category) int { return 1 }
	l := ratelimit.New(loader)

	l.Allow("t", ratelimit.CategoryQuery) // exhaust
	l.Evict("t")

	// After eviction, a new bucket is created on the next Allow — starts full.
	if !l.Allow("t", ratelimit.CategoryQuery) {
		t.Error("after Evict, first request should succeed (fresh bucket)")
	}
}

func TestAllow_AllCategoriesExist(t *testing.T) {
	loader := func(_ string, _ ratelimit.Category) int { return 5 }
	l := ratelimit.New(loader)

	cats := []ratelimit.Category{
		ratelimit.CategoryIngest,
		ratelimit.CategoryQuery,
		ratelimit.CategoryAdmin,
		ratelimit.CategoryMutation,
	}
	for _, cat := range cats {
		if !l.Allow("t", cat) {
			t.Errorf("category %q: first request should be allowed", cat)
		}
	}
}
