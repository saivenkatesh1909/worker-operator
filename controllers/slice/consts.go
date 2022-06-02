package slice

const (
	ControlPlaneNamespace                      = "kubeslice-system"
	ApplicationNamespaceSelectorLabelKey       = "kubeslice.io/slice"
	AllowedNamespaceSelectorLabelKey           = "kubeslice.io/namespace"
	sliceBakendUpdateInterval            int64 = 60
)
