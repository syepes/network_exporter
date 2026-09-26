package config

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

// TestResolverCacheDedupesWithinTTL verifies that repeated resolutions of the same
// host+family within the TTL window collapse to a single underlying lookup, which
// is the core win of the DNS cache during a refresh round (AddTargets resolves the
// same host twice, and DelTargets/CheckActiveTargets resolve it again).
func TestResolverCacheDedupesWithinTTL(t *testing.T) {
	calls := 0
	lookup := func(_ context.Context, host string, _ *net.Resolver, _ time.Duration, _ bool) ([]string, error) {
		calls++
		return []string{"1.2.3.4"}, nil
	}

	r := &Resolver{TTL: time.Minute}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		addrs, err := r.resolve(ctx, "example.com", false, lookup)
		if err != nil {
			t.Fatalf("resolve #%d: unexpected error: %v", i, err)
		}
		if len(addrs) != 1 || addrs[0] != "1.2.3.4" {
			t.Fatalf("resolve #%d: unexpected addrs: %v", i, addrs)
		}
	}

	if calls != 1 {
		t.Fatalf("expected 1 underlying lookup, got %d", calls)
	}
}

// TestResolverCacheSeparatesFamilies verifies IPv4 and IPv6 results for the same
// host are cached under distinct keys and never collide.
func TestResolverCacheSeparatesFamilies(t *testing.T) {
	lookup := func(_ context.Context, host string, _ *net.Resolver, _ time.Duration, enableIPv6 bool) ([]string, error) {
		if enableIPv6 {
			return []string{"::1"}, nil
		}
		return []string{"127.0.0.1"}, nil
	}

	r := &Resolver{TTL: time.Minute}
	ctx := context.Background()

	v4, _ := r.resolve(ctx, "localhost", false, lookup)
	v6, _ := r.resolve(ctx, "localhost", true, lookup)

	if len(v4) != 1 || v4[0] != "127.0.0.1" {
		t.Fatalf("IPv4 resolve returned %v", v4)
	}
	if len(v6) != 1 || v6[0] != "::1" {
		t.Fatalf("IPv6 resolve returned %v", v6)
	}
}

// TestResolverCacheExpires verifies that once the TTL elapses the next resolution
// performs a fresh lookup, so DNS record changes are eventually picked up.
func TestResolverCacheExpires(t *testing.T) {
	calls := 0
	lookup := func(_ context.Context, host string, _ *net.Resolver, _ time.Duration, _ bool) ([]string, error) {
		calls++
		return []string{"1.2.3.4"}, nil
	}

	r := &Resolver{TTL: 10 * time.Millisecond}
	ctx := context.Background()

	if _, err := r.resolve(ctx, "example.com", false, lookup); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := r.resolve(ctx, "example.com", false, lookup); err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	if calls != 2 {
		t.Fatalf("expected 2 lookups after TTL expiry, got %d", calls)
	}
}

// TestResolverDoesNotCacheErrors verifies a transient DNS failure is not pinned in
// the cache: a failing lookup followed by a succeeding one must not serve the
// error, and both must reach the underlying lookup.
func TestResolverDoesNotCacheErrors(t *testing.T) {
	calls := 0
	lookup := func(_ context.Context, host string, _ *net.Resolver, _ time.Duration, _ bool) ([]string, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient failure")
		}
		return []string{"1.2.3.4"}, nil
	}

	r := &Resolver{TTL: time.Minute}
	ctx := context.Background()

	if _, err := r.resolve(ctx, "example.com", false, lookup); err == nil {
		t.Fatal("expected error on first resolve")
	}
	addrs, err := r.resolve(ctx, "example.com", false, lookup)
	if err != nil {
		t.Fatalf("second resolve: unexpected error: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != "1.2.3.4" {
		t.Fatalf("second resolve returned %v", addrs)
	}
	if calls != 2 {
		t.Fatalf("expected 2 lookups (error not cached), got %d", calls)
	}
}

// TestResolverTTLZeroDisablesCache verifies TTL <= 0 disables caching entirely so
// every call reaches the underlying lookup.
func TestResolverTTLZeroDisablesCache(t *testing.T) {
	calls := 0
	lookup := func(_ context.Context, host string, _ *net.Resolver, _ time.Duration, _ bool) ([]string, error) {
		calls++
		return []string{"1.2.3.4"}, nil
	}

	r := &Resolver{TTL: 0}
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := r.resolve(ctx, "example.com", false, lookup); err != nil {
			t.Fatalf("resolve #%d: %v", i, err)
		}
	}
	if calls != 3 {
		t.Fatalf("expected 3 lookups with caching disabled, got %d", calls)
	}
}
