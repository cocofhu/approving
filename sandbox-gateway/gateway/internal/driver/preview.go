package driver

import "strconv"

const (
	EnvPreviewDirect      = "PREVIEW_DIRECT"
	EnvPreviewPort        = "PREVIEW_PORT"
	EnvPreviewPublicURL   = "PREVIEW_PUBLIC_URL"
	PreviewPortMin        = 18080
	PreviewPortMax        = 18999
	DefaultK8sPreviewPort = 18080
)

// PreviewDirectEnabled reports whether the sandbox requested IP-direct app preview.
func PreviewDirectEnabled(env map[string]string) bool {
	return env != nil && env[EnvPreviewDirect] == "1"
}

// AppendPort adds p to ports if it is a valid unused TCP port.
func AppendPort(ports []int, p int) []int {
	if p < 1 || p > 65535 {
		return ports
	}
	for _, x := range ports {
		if x == p {
			return ports
		}
	}
	return append(ports, p)
}

// ApplyK8sPreviewDirect publishes a same-number Service/LB port when PREVIEW_DIRECT=1.
// Each sandbox has its own ExternalIP, so the default 18080 does not collide.
func ApplyK8sPreviewDirect(spec *Spec) {
	if spec == nil || !PreviewDirectEnabled(spec.Env) {
		return
	}
	p := DefaultK8sPreviewPort
	if raw := spec.Env[EnvPreviewPort]; raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 65535 {
			p = n
		}
	}
	spec.Env[EnvPreviewPort] = strconv.Itoa(p)
	spec.Ports = AppendPort(spec.Ports, p)
}
