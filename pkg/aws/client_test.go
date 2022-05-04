package aws

import (
	"context"
	"log"
	"testing"

	"domc.me/irsa-controller/api/v1alpha1"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/elgohr/go-localstack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testClusterName     = "test-cluster"
	testIamRolePrefix   = "test-iam-role"
	testOidcProviderArn = "test-oidc"

	testManagedPolicyName = "managedPolicy"
)

func TestIamClient_Create(t *testing.T) {
	l, err := localstack.NewInstance()
	if err != nil {
		t.Fatalf("Could not connect to Docker %v", err)
	}
	if err := l.Start(); err != nil {
		t.Fatalf("Could not start localstack %v", err)
	}
	defer l.Stop()
	client := getMockIamClient(l)
	managed, err := client.iamClient.CreatePolicy(&iam.CreatePolicyInput{
		PolicyName:     aws.String(testManagedPolicyName),
		PolicyDocument: aws.String(`{"Version":"2012-10-17","Statement":[{"Resource":"*","Effect":"Allow","Action":"*"}]}`),
	})
	if err != nil {
		t.Fatalf("Create managed policy in test cases failed: %v", err)
	}

	type args struct {
		ctx          context.Context
		oidcProvider string
		irsa         *v1alpha1.IamRoleServiceAccount
	}
	tests := []struct {
		name    string
		args    args
		want    string
		want1   string
		wantErr bool
	}{
		{
			name: "create with manged polices and inline policy",
			args: args{
				ctx:          context.Background(),
				oidcProvider: testOidcProviderArn,
				irsa: &v1alpha1.IamRoleServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "iam-role-1",
						Namespace: "default",
					},
					Spec: v1alpha1.IamRoleServiceAccountSpec{
						Policy: &v1alpha1.PolicySpec{
							ManagedPolicies: []string{
								*managed.Policy.Arn,
							},
							InlinePolicy: &v1alpha1.InlinePolicySpec{
								Version: "2012-10-17",
								Statements: []v1alpha1.StatementSpec{
									{
										Resource: []string{
											"*",
										},
										Action: []string{
											"iam:*",
										},
										Effect: "Allow",
									},
								},
							},
						},
					},
				},
			},
			want:  "arn:aws:iam::000000000000:role/test-iam-role-test-cluster-default-iam-role-1",
			want1: "arn:aws:iam::000000000000:policy/test-iam-role-test-cluster-default-iam-role-1-inline-policy",
		},
		{
			name: "create with inline policy",
			args: args{
				ctx:          context.Background(),
				oidcProvider: testOidcProviderArn,
				irsa: &v1alpha1.IamRoleServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "iam-role-2",
						Namespace: "default",
					},
					Spec: v1alpha1.IamRoleServiceAccountSpec{
						Policy: &v1alpha1.PolicySpec{
							InlinePolicy: &v1alpha1.InlinePolicySpec{
								Version: "2012-10-17",
								Statements: []v1alpha1.StatementSpec{
									{
										Resource: []string{
											"*",
										},
										Action: []string{
											"iam:*",
										},
										Effect: "Allow",
									},
								},
							},
						},
					},
				},
			},
			want:  "arn:aws:iam::000000000000:role/test-iam-role-test-cluster-default-iam-role-2",
			want1: "arn:aws:iam::000000000000:policy/test-iam-role-test-cluster-default-iam-role-2-inline-policy",
		},
		{
			name: "create with managed policy",
			args: args{
				ctx:          context.Background(),
				oidcProvider: testOidcProviderArn,
				irsa: &v1alpha1.IamRoleServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "iam-role-3",
						Namespace: "default",
					},
					Spec: v1alpha1.IamRoleServiceAccountSpec{
						Policy: &v1alpha1.PolicySpec{
							ManagedPolicies: []string{
								*managed.Policy.Arn,
							},
						},
					},
				},
			},
			want: "arn:aws:iam::000000000000:role/test-iam-role-test-cluster-default-iam-role-3",
		},
		{
			name: "create with iam role already exists",
			args: args{
				ctx:          context.Background(),
				oidcProvider: testOidcProviderArn,
				irsa: &v1alpha1.IamRoleServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "iam-role-1",
						Namespace: "default",
					},
					Spec: v1alpha1.IamRoleServiceAccountSpec{
						Policy: &v1alpha1.PolicySpec{
							ManagedPolicies: []string{
								*managed.Policy.Arn,
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client
			got, got1, err := c.Create(tt.args.ctx, tt.args.oidcProvider, tt.args.irsa)
			if (err != nil) != tt.wantErr {
				t.Errorf("IamClient.Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IamClient.Create() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("IamClient.Create() got1 = %v, want %v", got1, tt.want1)
			}

			if got != "" {
				role, err := c.iamClient.GetRole(&iam.GetRoleInput{
					RoleName: aws.String(RoleNameByArn(got)),
				})
				if err != nil {
					t.Errorf("Get iam role %s failed: %v", got, err)
				}
				wantAssumeRoleDoc, err := NewAssumeRolePolicyDoc(testOidcProviderArn, tt.args.irsa.GetNamespace(), tt.args.irsa.GetName())
				if err != nil {
					t.Errorf("New assume role policy failed: %v", err)
				}
				if *role.Role.AssumeRolePolicyDocument != wantAssumeRoleDoc {
					t.Errorf("The assume role document of iam role got = %s, want: %s", *role.Role.AssumeRolePolicyDocument, wantAssumeRoleDoc)
				}
				policies, err := c.iamClient.ListAttachedRolePolicies(&iam.ListAttachedRolePoliciesInput{
					RoleName: aws.String(RoleNameByArn(got)),
				})
				if err != nil {
					t.Errorf("List attached role policies in %s failed: %v", got, err)
				}

				findGot1 := false
				if got1 == "" {
					findGot1 = true
				}
				for _, ap := range policies.AttachedPolicies {
					// check inline policy
					if *ap.PolicyArn == got1 {
						findGot1 = true
					} else if tt.args.irsa.Spec.Policy != nil {
						findMp := false
						// check manged polices
						for _, mp := range tt.args.irsa.Spec.Policy.ManagedPolicies {
							if mp == *ap.PolicyArn {
								findMp = true
							}
						}
						if !findMp {
							t.Errorf("Iam roles contains unknown policy arn %s", *ap.PolicyArn)
						}
					}

				}

				if !findGot1 {
					t.Errorf("Inline policy %s in not in attached policies: %v", got1, policies.AttachedPolicies)
				}
			}
		})
	}
}

func getMockIamClient(l *localstack.Instance) *IamClient {
	configurationForTest, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Endpoint:    aws.String(l.Endpoint(localstack.IAM)),
		DisableSSL:  aws.Bool(true),
		Credentials: credentials.NewStaticCredentials("not", "empty", ""),
	})
	if err != nil {
		log.Fatalf("Cloud not get configuration from localstack %v", err)
	}
	return newIamClient(testClusterName, testIamRolePrefix, []string{}, iam.New(configurationForTest))
}

func TestIamClient_RoleName(t *testing.T) {
	type fields struct {
		prefix      string
		clusterName string
	}
	type args struct {
		irsa *v1alpha1.IamRoleServiceAccount
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "Expect",
			fields: fields{
				clusterName: "cls",
				prefix:      "pre",
			},
			args: args{
				irsa: &v1alpha1.IamRoleServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "name",
						Namespace: "ns",
					},
				},
			},
			want: "pre-cls-ns-name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &IamClient{
				prefix:      tt.fields.prefix,
				clusterName: tt.fields.clusterName,
			}
			if got := c.RoleName(tt.args.irsa); got != tt.want {
				t.Errorf("IamClient.RoleName() = %v, want %v", got, tt.want)
			}
		})
	}
}
