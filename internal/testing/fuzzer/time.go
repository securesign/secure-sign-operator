package fuzzer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/randfill"
)

func Time(input **metav1.Time, c randfill.Continue) {
	if c.Bool() {
		*input = nil
		return
	}
	var sec, nsec uint32
	c.Fill(&sec)
	c.Fill(&nsec)
	t := metav1.Unix(int64(sec), int64(nsec)).Rfc3339Copy()
	*input = &t
}
