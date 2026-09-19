package clipboard

// Arbitrate implements the DEC-023 reconnect and conflict arbitration rules.
//
// Rules:
//   - If both local and remote states are empty: WinnerNone (no-op).
//   - If local is empty and remote is not: WinnerRemote.
//   - If remote is empty and local is not: WinnerLocal.
//   - If both states are non-empty and their SHA-256 digests match: WinnerNone (no-op, identical content).
//   - If digests differ:
//     1. If remote copied_at_ms > local copied_at_ms + 1000 ms: WinnerRemote (remote is strictly newer).
//     2. If local copied_at_ms > remote copied_at_ms + 1000 ms: WinnerLocal (local is strictly newer).
//     3. If |remote - local| <= 1000 ms (or equal, or zero): Linux/Desktop wins as a deterministic tie-breaker.
//     If role is RoleDesktop, returns WinnerLocal; if role is RoleMobile, returns WinnerRemote.
func Arbitrate(local *Item, remote *Item, role Role) Winner {
	localEmpty := local.IsEmpty()
	remoteEmpty := remote.IsEmpty()

	if localEmpty && remoteEmpty {
		return WinnerNone
	}
	if localEmpty && !remoteEmpty {
		return WinnerRemote
	}
	if !localEmpty && remoteEmpty {
		return WinnerLocal
	}

	// Both are non-empty: if digests match, content is identical -> no-op.
	if local.Digest == remote.Digest {
		return WinnerNone
	}

	localTs := local.CopiedAtMs
	remoteTs := remote.CopiedAtMs

	// Overflow-safe timestamp comparison
	if remoteTs > localTs && remoteTs-localTs > ArbitrationThresholdMs {
		return WinnerRemote
	}
	if localTs > remoteTs && localTs-remoteTs > ArbitrationThresholdMs {
		return WinnerLocal
	}

	// Difference is <= 1000 ms: Desktop wins tie-breaker.
	if role == RoleDesktop {
		return WinnerLocal
	}
	return WinnerRemote
}
