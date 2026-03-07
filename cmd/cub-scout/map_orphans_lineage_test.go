package main

import "testing"

func TestIsMapOrphanCandidate(t *testing.T) {
	tests := []struct {
		name string
		in   MapEntry
		want bool
	}{
		{
			name: "native orphan",
			in: MapEntry{
				Owner: "Native",
			},
			want: true,
		},
		{
			name: "argocd appset orphan",
			in: MapEntry{
				Kind:  "Application",
				Owner: "ArgoCD",
				OwnerDetails: map[string]string{
					"applicationSetLinkStatus": "orphan",
				},
			},
			want: true,
		},
		{
			name: "argocd appset resolved",
			in: MapEntry{
				Kind:  "Application",
				Owner: "ArgoCD",
				OwnerDetails: map[string]string{
					"applicationSetLinkStatus": "resolved",
				},
			},
			want: false,
		},
		{
			name: "argocd non-application",
			in: MapEntry{
				Kind:  "Deployment",
				Owner: "ArgoCD",
				OwnerDetails: map[string]string{
					"applicationSetLinkStatus": "orphan",
				},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMapOrphanCandidate(tc.in); got != tc.want {
				t.Fatalf("isMapOrphanCandidate() = %v, want %v (entry=%+v)", got, tc.want, tc.in)
			}
		})
	}
}
