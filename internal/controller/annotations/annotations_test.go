package annotations

import (
	"maps"
	"reflect"
	"strconv"
	"testing"
)

func TestFilterInheritable(t *testing.T) {
	allIn := make(map[string]string)
	allCopy := make(map[string]string)
	for i, a := range inheritable {
		allIn[a] = strconv.FormatInt(int64(i), 10)
	}
	maps.Copy(allCopy, allIn)

	type args struct {
		annotations map[string]string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "empty",
			args: args{
				annotations: make(map[string]string),
			},
			want: map[string]string{},
		},
		{
			name: "nil",
			args: args{
				annotations: nil,
			},
			want: map[string]string{},
		},
		{
			name: "no inheritable",
			args: args{
				annotations: map[string]string{
					"name1": "value",
					"name2": "value",
				},
			},
			want: map[string]string{},
		},
		{
			name: "all inheritable",
			args: args{
				annotations: allIn,
			},
			want: allCopy,
		},
		{
			name: "one inheritable",
			args: args{
				annotations: map[string]string{
					"name1":   "value",
					"name2":   "value",
					TrustedCA: "ca",
				},
			},
			want: map[string]string{
				TrustedCA: "ca",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FilterInheritable(tt.args.annotations); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterInheritable() = %v, want %v", got, tt.want)
			}
		})
	}
}
