/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gcemodel

import (
	"fmt"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/wellknownports"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/gcetasks"
)

// APILoadBalancerBuilder builds a LoadBalancer for accessing the API
type APILoadBalancerBuilder struct {
	*GCEModelContext
	Lifecycle fi.Lifecycle
}

var _ fi.ModelBuilder = &APILoadBalancerBuilder{}

func (b *APILoadBalancerBuilder) Build(c *fi.ModelBuilderContext) error {
	if !b.UseLoadBalancerForAPI() {
		return nil
	}

	lbSpec := b.Cluster.Spec.API.LoadBalancer
	if lbSpec == nil {
		// Skipping API LB creation; not requested in Spec
		return nil
	}

	switch lbSpec.Type {
	case kops.LoadBalancerTypePublic:
	// OK

	case kops.LoadBalancerTypeInternal:
		return fmt.Errorf("internal LoadBalancers are not yet supported by kops on GCE")

	default:
		return fmt.Errorf("unhandled LoadBalancer type %q", lbSpec.Type)
	}

	healthCheck := &gcetasks.Healthcheck{
		Name:      s(b.NameForHealthcheck("api")),
		Port:      i64(wellknownports.KubeAPIServerHealthCheck),
		Lifecycle: b.Lifecycle,
	}

	c.AddTask(healthCheck)

	targetPool := &gcetasks.TargetPool{
		Name:      s(b.NameForTargetPool("api")),
		Lifecycle: b.Lifecycle,
	}
	c.AddTask(targetPool)

	poolHealthCheck := &gcetasks.PoolHealthCheck{
		Name:        s(b.NameForPoolHealthcheck("api")),
		Healthcheck: healthCheck,
		Pool:        targetPool,
		Lifecycle:   b.Lifecycle,
	}
	c.AddTask(poolHealthCheck)

	ipAddress := &gcetasks.Address{
		Name:      s(b.NameForIPAddress("api")),
		Lifecycle: b.Lifecycle,
	}
	c.AddTask(ipAddress)

	forwardingRule := &gcetasks.ForwardingRule{
		Name:       s(b.NameForForwardingRule("api")),
		Lifecycle:  b.Lifecycle,
		PortRange:  "443-443",
		TargetPool: targetPool,
		IPAddress:  ipAddress,
		IPProtocol: "TCP",
	}
	// TODO: Health check
	c.AddTask(forwardingRule)

	{
		// Ensure the IP address is included in our certificate
		ipAddress.ForAPIServer = true
	}

	// Allow traffic into the API (port 443) from KubernetesAPIAccess CIDRs
	{
		b.AddFirewallRulesTasks(c, "https-api", &gcetasks.FirewallRule{
			Lifecycle:    b.Lifecycle,
			Network:      b.LinkToNetwork(),
			SourceRanges: b.Cluster.Spec.KubernetesAPIAccess,
			TargetTags:   []string{b.GCETagForRole(kops.InstanceGroupRoleMaster)},
			Allowed:      []string{"tcp:443"},
		})
	}
	return nil
}
