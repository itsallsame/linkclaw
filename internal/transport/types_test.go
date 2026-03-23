package transport

import "testing"

func TestRouteTypeBoundaryHelpers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		routeType RouteType
		known     bool
		p0        bool
		reserved  bool
	}{
		{routeType: RouteTypeDirect, known: true, p0: true, reserved: false},
		{routeType: RouteTypeStoreForward, known: true, p0: true, reserved: false},
		{routeType: RouteTypeRecovery, known: true, p0: true, reserved: false},
		{routeType: RouteTypeNostr, known: true, p0: false, reserved: true},
		{routeType: RouteType("custom"), known: false, p0: false, reserved: false},
	}

	for _, tc := range cases {
		if got := IsKnownRouteType(tc.routeType); got != tc.known {
			t.Fatalf("IsKnownRouteType(%q) = %v, want %v", tc.routeType, got, tc.known)
		}
		if got := IsP0RouteType(tc.routeType); got != tc.p0 {
			t.Fatalf("IsP0RouteType(%q) = %v, want %v", tc.routeType, got, tc.p0)
		}
		if got := IsReservedRouteType(tc.routeType); got != tc.reserved {
			t.Fatalf("IsReservedRouteType(%q) = %v, want %v", tc.routeType, got, tc.reserved)
		}
	}
}

func TestRouteCandidateIsP0(t *testing.T) {
	t.Parallel()

	if !(RouteCandidate{Type: RouteTypeDirect}).IsP0() {
		t.Fatal("direct route candidate should be treated as P0")
	}
	if (RouteCandidate{Type: RouteTypeNostr}).IsP0() {
		t.Fatal("nostr route candidate should not be treated as P0")
	}
}
