
<b>Pattern 1: Always initialize Gomega within each t.Run subtest to ensure failures are scoped to the subtest and do not terminate sibling tests.</b>

Example code before:
```
func TestFoo(t *testing.T) {
  g := NewWithT(t)
  for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) {
      g.Expect(Compute(tc.in)).To(Equal(tc.want)) // shares parent g; failNow aborts siblings
    })
  }
}
```

Example code after:
```
func TestFoo(t *testing.T) {
  for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) {
      g := NewWithT(t) // create per-subtest
      g.Expect(Compute(tc.in)).To(Equal(tc.want))
    })
  }
}
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/securesign/secure-sign-operator/pull/1181#discussion_r2191839409
</details>


___

<b>Pattern 2: Add and enforce default timeouts in Ginkgo/Gomega E2E tests by calling EnforceDefaultTimeoutsWhenUsingContexts and setting SetDefaultEventuallyTimeout to avoid hanging Eventually blocks.</b>

Example code before:
```
func TestE2E(t *testing.T) {
  RegisterFailHandler(Fail)
  RunSpecs(t, "Suite")
  // Eventually(...) without a default timeout may hang
}
```

Example code after:
```
func TestE2E(t *testing.T) {
  RegisterFailHandler(Fail)
  SetDefaultEventuallyTimeout(3 * time.Minute)
  EnforceDefaultTimeoutsWhenUsingContexts()
  RunSpecs(t, "Suite")
}
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/securesign/secure-sign-operator/pull/1298#discussion_r2314147484
</details>


___

<b>Pattern 3: Avoid unnecessary per-iteration allocations in loops by constructing shared slices or data structures once before the loop and reusing them inside.</b>

Example code before:
```
func Apply(names ...string) {
  for i, c := range pod.Spec.Containers {
    sel := append(names[:0:0], names...) // or recomputing inside loop
    if contains(sel, c.Name) { /* ... */ }
  }
}
```

Example code after:
```
func Apply(names ...string) {
  sel := append([]string(nil), names...) // build once
  for i, c := range pod.Spec.Containers {
    if contains(sel, c.Name) { /* ... */ }
  }
}
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/securesign/secure-sign-operator/pull/1200#discussion_r2212645370
</details>


___

<b>Pattern 4: Provide unit tests for API validation rules and controller edge cases when introducing CEL validations or complex condition handling, covering all options and immutability constraints.</b>

Example code before:
```
// Add CEL rule, but no tests
// +kubebuilder:validation:XValidation:rule=(self || !oldSelf)
Enabled bool `json:"enabled"`
```

Example code after:
```
func TestMonitoring_Immutability(t *testing.T) {
  // create resource with Enabled=true
  // attempt to update to false and assert IsInvalid error and message
}
func TestHandler_AllOptions(t *testing.T) {
  // exercise nil/empty fields (e.g., optional Port), all branches and ensure consistent URLs/status
}
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/securesign/secure-sign-operator/pull/1143#discussion_r2142898824
- https://github.com/securesign/secure-sign-operator/pull/1116#discussion_r2111714336
- https://github.com/securesign/secure-sign-operator/pull/1200#discussion_r2212648574
- https://github.com/securesign/secure-sign-operator/pull/1128#discussion_r2215880911
</details>


___
