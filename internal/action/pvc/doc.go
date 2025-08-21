/*
Package pvc implements a generic action for managing [k8s.io/api/core/v1.PersistentVolumeClaim] (PVCs).

This action handles the lifecycle of a PVC associated with a custom resource. It can create,
update, or discover a PVC based on the specification within the custom resource. The primary
goal is to abstract and standardize the logic for persistent storage provisioning within
the operator's reconciliation loop, ensuring that stateful components have their required
storage reliably managed.

The action is implemented as a generic type to work with any custom resource that satisfies
the [apis.ConditionsAwareObject] interface. It uses a wrapper to abstract the details of
how to access the PVC specification and status fields from the custom resource object.

Workflow:
 1. Check if an explicit PVC name is provided in the resource's spec. If so, adopt that
    PVC, update the status, and the process is complete.
 2. If no PVC name is in the status, attempt to discover an existing PVC with a default name.
    If found, adopt it and update the status.
 3. If no PVC is specified or discovered, proceed to create or update a PVC.
    a. Validate that the PVC size is specified in the spec; if not, the reconciliation fails with a terminal error.
    b. Create or update the PVC with the specified size, access modes, and storage class.
    c. Set ownership labels and an optional controller reference on the PVC, unless the retain policy is enabled.
 4. Update the custom resource's status with the name of the managed PVC and a condition
    reflecting the outcome (e.g., Created, Updated, Discovered, Specified).

Usage:

	// First, define a wrapper that provides access to the custom resource's PVC fields.
	pvcWrapper := pvc.Wrapper[*v1alpha1.Rekor](
	    func(r *v1alpha1.Rekor) v1alpha1.Pvc {
	        return r.Spec.Pvc
	    },
	    func(r *v1alpha1.Rekor) string {
	        return r.Status.PvcName
	    },
	    func(r *v1alpha1.Rekor, s string) {
	        r.Status.PvcName = s
	    },
	    // This function determines if the PVC action should run.
	    func(r *v1alpha1.Rekor) bool {
	        return true
	    },
	)

	// Then, create the action with a name format and component details.
	pvc.NewAction[*v1alpha1.Rekor]("rekor-pvc", "rekor", "deployment", pvcWrapper)
*/
package pvc
