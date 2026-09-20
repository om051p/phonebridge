package clipboard

import "bytes"

// Arbitrate implements the DEC-023 reconnect and conflict arbitration rules.
// For asymmetric roles (RoleDesktop vs RoleMobile), Desktop wins ties per DEC-023.
func Arbitrate(local *Item, remote *Item, role Role) Winner {
	remoteRole := RoleMobile
	if role == RoleMobile {
		remoteRole = RoleDesktop
	}
	return ArbitratePeer(local, remote, role, remoteRole, "", "")
}

// ArbitratePeer implements symmetric and asymmetric reconnect and conflict arbitration.
//
// Rules:
//   - If both local and remote states are empty: WinnerNone (no-op).
//   - If local is empty and remote is not: WinnerRemote.
//   - If remote is empty and local is not: WinnerLocal.
//   - If both states are non-empty and their SHA-256 digests match: WinnerNone (no-op, identical content).
//   - If digests differ:
//     1. If remote copied_at_ms > local copied_at_ms + 1000 ms: WinnerRemote (remote is strictly newer).
//     2. If local copied_at_ms > remote copied_at_ms + 1000 ms: WinnerLocal (local is strictly newer).
//     3. If |remote - local| <= 1000 ms (or equal, or zero):
//        a. If roles differ: RoleDesktop wins as a deterministic tie-breaker (DEC-023).
//        b. If roles are identical (e.g. Linux <-> Linux or Mobile <-> Mobile):
//           Tie is broken deterministically by lexicographically comparing local and remote
//           peer IDs (if non-empty) or item SHA-256 digests.
func ArbitratePeer(local *Item, remote *Item, localRole, remoteRole Role, localPeerID, remotePeerID string) Winner {
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

	// Difference is <= 1000 ms: tie-breaker.
	if localRole != remoteRole {
		if localRole == RoleDesktop {
			return WinnerLocal
		}
		return WinnerRemote
	}

	// Identical roles: deterministic tie-breaker.
	if localPeerID != "" && remotePeerID != "" && localPeerID != remotePeerID {
		if localPeerID > remotePeerID {
			return WinnerLocal
		}
		return WinnerRemote
	}

	// Fallback to digest comparison (always non-zero because digests differ).
	if bytes.Compare(local.Digest[:], remote.Digest[:]) > 0 {
		return WinnerLocal
	}
	return WinnerRemote
}
