package controllers

import (
	"context"
	"strings"
	"time"

	federationv1 "github.com/cloudfunny/kubefederation/api/v1"
	"github.com/cloudfunny/kubefederation/api/v1/common"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	kubeclientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	UserAgentName = "Cluster-Controller"

	KubeAPIQPS        = 20.0
	KubeAPIBurst      = 30
	TokenKey          = "token"
	CaCrtKey          = "ca.crt"
	KubeFedConfigName = "kubefed"

	// Common ClusterConditions for KubeFedClusterStatus
	ClusterReady                 = "ClusterReady"
	HealthzOk                    = "/healthz responded with ok"
	ClusterNotReady              = "ClusterNotReady"
	HealthzNotOk                 = "/healthz responded without ok"
	ClusterNotReachableReason    = "ClusterNotReachable"
	ClusterNotReachableMsg       = "cluster is not reachable"
	ClusterReachableReason       = "ClusterReachable"
	ClusterReachableMsg          = "cluster is reachable"
	ClusterConfigMalformedReason = "ClusterConfigMalformed"
	ClusterConfigMalformedMsg    = "cluster's configuration may be malformed"
)

type ClusterClient struct {
	kubeClient  *kubeclientset.Clientset
	clusterName string
}

func NewClusterClientSet(c *federationv1.FederatedCluster, client client.Client, timeout time.Duration) (*ClusterClient, error) {
	clusterClientSet := ClusterClient{clusterName: c.Name}
	clusterConfig, err := buildClusterConfig(c, client)
	if err != nil {
		return &clusterClientSet, err
	}
	clusterConfig.Timeout = timeout
	clusterClientSet.kubeClient, err = kubeclientset.NewForConfig(restclient.AddUserAgent(clusterConfig, UserAgentName))
	return &clusterClientSet, err
}

func buildClusterConfig(fedCluster *federationv1.FederatedCluster, client client.Client) (*restclient.Config, error) {
	clusterName := fedCluster.Name

	apiEndpoint := fedCluster.Spec.APIEndpoint
	// TODO(marun) Remove when validation ensures a non-empty value.
	if apiEndpoint == "" {
		return nil, errors.Errorf("The api endpoint of cluster %s is empty", clusterName)
	}

	secretName := fedCluster.Spec.SecretRef.Name
	if secretName == "" {
		return nil, errors.Errorf("Cluster %s does not have a secret name", clusterName)
	}
	secret := &apiv1.Secret{}
	keyObj := types.NamespacedName{
		Namespace: fedCluster.Namespace,
		Name:      fedCluster.Spec.SecretRef.Name,
	}
	err := client.Get(context.TODO(), keyObj, secret)
	if err != nil {
		return nil, err
	}

	token, tokenFound := secret.Data[TokenKey]
	if !tokenFound || len(token) == 0 {
		return nil, errors.Errorf("The secret for cluster %s is missing a non-empty value for %q", clusterName, TokenKey)
	}

	clusterConfig, err := clientcmd.BuildConfigFromFlags(apiEndpoint, "")
	if err != nil {
		return nil, err
	}
	clusterConfig.CAData = fedCluster.Spec.CABundle
	clusterConfig.BearerToken = string(token)
	clusterConfig.QPS = KubeAPIQPS
	clusterConfig.Burst = KubeAPIBurst

	return clusterConfig, nil
}

// GetClusterHealthStatus gets the kubernetes cluster health status by requesting "/healthz"
func (c *ClusterClient) GetClusterHealthStatus() (*federationv1.FederatedClusterStatus, error) {
	clusterStatus := federationv1.FederatedClusterStatus{}
	currentTime := metav1.Now()
	clusterReady := ClusterReady
	healthzOk := HealthzOk
	newClusterReadyCondition := federationv1.ClusterCondition{
		Type:               common.ClusterReady,
		Status:             apiv1.ConditionTrue,
		Reason:             &clusterReady,
		Message:            &healthzOk,
		LastProbeTime:      currentTime,
		LastTransitionTime: &currentTime,
	}
	clusterNotReady := ClusterNotReady
	healthzNotOk := HealthzNotOk
	newClusterNotReadyCondition := federationv1.ClusterCondition{
		Type:               common.ClusterReady,
		Status:             apiv1.ConditionFalse,
		Reason:             &clusterNotReady,
		Message:            &healthzNotOk,
		LastProbeTime:      currentTime,
		LastTransitionTime: &currentTime,
	}
	clusterNotReachableReason := ClusterNotReachableReason
	clusterNotReachableMsg := ClusterNotReachableMsg
	newClusterOfflineCondition := federationv1.ClusterCondition{
		Type:               common.ClusterOffline,
		Status:             apiv1.ConditionTrue,
		Reason:             &clusterNotReachableReason,
		Message:            &clusterNotReachableMsg,
		LastProbeTime:      currentTime,
		LastTransitionTime: &currentTime,
	}
	clusterReachableReason := ClusterReachableReason
	clusterReachableMsg := ClusterReachableMsg
	newClusterNotOfflineCondition := federationv1.ClusterCondition{
		Type:               common.ClusterOffline,
		Status:             apiv1.ConditionFalse,
		Reason:             &clusterReachableReason,
		Message:            &clusterReachableMsg,
		LastProbeTime:      currentTime,
		LastTransitionTime: &currentTime,
	}
	clusterConfigMalformedReason := ClusterConfigMalformedReason
	clusterConfigMalformedMsg := ClusterConfigMalformedMsg
	newClusterConfigMalformedCondition := federationv1.ClusterCondition{
		Type:               common.ClusterConfigMalformed,
		Status:             apiv1.ConditionTrue,
		Reason:             &clusterConfigMalformedReason,
		Message:            &clusterConfigMalformedMsg,
		LastProbeTime:      currentTime,
		LastTransitionTime: &currentTime,
	}
	if c.kubeClient == nil {
		clusterStatus.Conditions = append(clusterStatus.Conditions, newClusterConfigMalformedCondition)
		return &clusterStatus, nil
	}
	body, err := c.kubeClient.DiscoveryClient.RESTClient().Get().AbsPath("/healthz").Do(context.Background()).Raw()
	if err != nil {
		runtime.HandleError(errors.Wrapf(err, "Failed to do cluster health check for cluster %q", c.clusterName))
		clusterStatus.Conditions = append(clusterStatus.Conditions, newClusterOfflineCondition)
	} else {
		if !strings.EqualFold(string(body), "ok") {
			clusterStatus.Conditions = append(clusterStatus.Conditions, newClusterNotReadyCondition, newClusterNotOfflineCondition)
		} else {
			clusterStatus.Conditions = append(clusterStatus.Conditions, newClusterReadyCondition)
		}
	}

	return &clusterStatus, err
}
