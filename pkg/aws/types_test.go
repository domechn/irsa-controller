package aws

import (
	"reflect"
	"testing"

	irsav1alpha1 "domc.me/irsa-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewIamRole(t *testing.T) {
	type args struct {
		oidcProviderArn string
		irsa            *irsav1alpha1.IamRoleServiceAccount
	}
	tests := []struct {
		name string
		args args
		want *IamRole
	}{
		{
			name: "new a iam role from irsa",
			args: args{
				oidcProviderArn: testOidcProviderArn,
				irsa: &irsav1alpha1.IamRoleServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: irsav1alpha1.IamRoleServiceAccountSpec{
						Policy: &irsav1alpha1.PolicySpec{
							ManagedPolicies: []string{
								"policy1",
							},
							InlinePolicy: &irsav1alpha1.InlinePolicySpec{
								Version: "2012-10-17",
								Statements: []irsav1alpha1.StatementSpec{
									{
										Resource: []string{"*"},
										Action:   []string{"*"},
										Effect:   string(StatementAllow),
										Condition: irsav1alpha1.StatementConditionSpec{
											"StringEquals": map[string]string{
												"key": "value",
											},
										},
									},
								},
							},
						},
					},
					Status: irsav1alpha1.IamRoleServiceAccountStatus{
						RoleArn: "arn:aws:iam::000000000000:role/test",
					},
				},
			},
			want: &IamRole{
				RoleArn:  "arn:aws:iam::000000000000:role/test",
				RoleName: "test",
				InlinePolicy: &RoleDocument{
					Version: "2012-10-17",
					Statement: []RoleStatement{
						{
							Resource: []string{"*"},
							Action:   []string{"*"},
							Effect:   StatementAllow,
							Condition: StatementCondition{
								"StringEquals": map[string]string{
									"key": "value",
								},
							},
						},
					},
				},
				ManagedPolicies:  []string{"policy1"},
				AssumeRolePolicy: assumeRoleDocument2Pointer(NewAssumeRolePolicy(testOidcProviderArn, "default", "test")),
			},
		},
		{
			name: "irsa with rolename should not work",
			args: args{
				oidcProviderArn: testOidcProviderArn,
				irsa: &irsav1alpha1.IamRoleServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: irsav1alpha1.IamRoleServiceAccountSpec{
						RoleName: "test",
					},
					Status: irsav1alpha1.IamRoleServiceAccountStatus{
						RoleArn: "arn:aws:iam::000000000000:role/test",
					},
				},
			},
			want: &IamRole{
				RoleArn:          "arn:aws:iam::000000000000:role/test",
				RoleName:         "test",
				AssumeRolePolicy: assumeRoleDocument2Pointer(NewAssumeRolePolicy(testOidcProviderArn, "default", "test")),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewIamRole(tt.args.oidcProviderArn, tt.args.irsa); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewIamRole() = %v, want %v", got, tt.want)
			}
		})
	}
}

func assumeRoleDocument2Pointer(a AssumeRoleDocument) *AssumeRoleDocument {
	return &a
}
