/*
Package tls implements a generic action for resolving TLS configuration.

This action handles TLS resolution for any custom resource that satisfies
the [apis.ConditionsAwareObject] interface. It uses a wrapper to abstract
the details of how to access the TLS specification and status fields from
the custom resource object.

Workflow:

 1. If the user has provided a certificate reference in spec, copy it to status.
 2. On OpenShift, auto-provision a serving certificate by generating a secret
    reference using the configured name format.
 3. On vanilla Kubernetes with no user certificate, log that communication
    is insecure and leave status TLS empty.
 4. Set the resolved condition and persist the status.

The isEnabled parameter to [Wrapper] controls whether the action runs at all.
Pass nil to indicate the component is always enabled.

Usage:

	tlsAction.NewAction[*v1.Trillian](
	    actions.ServerCondition,
	    metav1.ConditionFalse,
	    actions.LogServerTLSSecret,
	    "trillian log server",
	    tlsAction.Wrapper(specTLS, statusTLS, setStatusTLS, nil),
	)
*/
package tls
