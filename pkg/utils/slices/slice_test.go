/*
Copyright 2022 domechn.

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

package slices

import (
	"reflect"
	"testing"
)

func TestContainsString(t *testing.T) {
	type args struct {
		slice []string
		s     string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Empty slice should return false",
			args: args{
				slice: []string{},
				s:     "test",
			},
			want: false,
		},
		{
			name: "Contains",
			args: args{
				slice: []string{"test", "test2"},
				s:     "test",
			},
			want: true,
		},
		{
			name: "Not contains",
			args: args{
				slice: []string{"test2"},
				s:     "test",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsString(tt.args.slice, tt.args.s); got != tt.want {
				t.Errorf("ContainsString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveString(t *testing.T) {
	type args struct {
		slice []string
		s     string
	}
	tests := []struct {
		name       string
		args       args
		wantResult []string
	}{
		{
			name: "Delete if found",
			args: args{
				slice: []string{"test", "test2"},
				s:     "test",
			},
			wantResult: []string{"test2"},
		},
		{
			name: "Not delete if not found",
			args: args{
				slice: []string{"test", "test2"},
				s:     "test3",
			},
			wantResult: []string{"test", "test2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotResult := RemoveString(tt.args.slice, tt.args.s); !reflect.DeepEqual(gotResult, tt.wantResult) {
				t.Errorf("RemoveString() = %v, want %v", gotResult, tt.wantResult)
			}
		})
	}
}

func TestEqual(t *testing.T) {
	type args struct {
		a []string
		b []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Equal",
			args: args{
				a: []string{"test", "test2"},
				b: []string{"test", "test2"},
			},
			want: true,
		},
		{
			name: "Not equal",
			args: args{
				a: []string{"test", "test2"},
				b: []string{"test"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Equal(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}
