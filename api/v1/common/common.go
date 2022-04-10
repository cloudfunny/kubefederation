package common

type ClusterConditionType string

// These are valid conditions of a cluster.
const (
	// ClusterReady means the cluster is ready to accept workloads.
	ClusterReady ClusterConditionType = "Ready"
	// ClusterOffline means the cluster is temporarily down or not reachable
	ClusterOffline ClusterConditionType = "Offline"
	// ClusterConfigMalformed means the cluster's configuration may be malformed.
	ClusterConfigMalformed ClusterConditionType = "ConfigMalformed"
)
