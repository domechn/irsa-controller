# irsa-controller

[![Docker](https://github.com/domechn/irsa-controller/actions/workflows/publish.yaml/badge.svg)](https://github.com/domechn/irsa-controller/actions/workflows/publish.yaml)

Using CRD to manage Kubernetes `ServiceAccount` and AWS `Iam Role`.

## Prerequisites

- EKS or Kubernetes With [amazon-eks-pod-identity-webhook](https://github.com/aws/amazon-eks-pod-identity-webhook) Installed
- Kubernetes Version >= 1.16

## Installation

### Kustomize

```shell
git clone https://github.com/domechn/irsa-controller
cd irsa-controller
# Update the configuration
vim config/manager/controller_manager_config.yaml
make deploy
```

Uninstallation

```shell
make undeploy
```

## Usage

### Use externally created IAM Role

Irsa-controller will add `AssumePolicy` to this iam role to allow the role to be used by the serviceAccount. And controller will not manage the permissions of this role. Users can dynamically change the permissions of this role through other tools such as `AWS Console` or `TerraForm`.

```yaml
apiVersion: irsa.domc.me/v1alpha1
kind: IamRoleServiceAccount
metadata:
  name: iamroleserviceaccount-sample
spec:
  roleName: <external-iam-role-name>
```

### Use CRD to define permissions for iam role

Irsa-controller will create an iam role on AWS based on the user-defined policy. The role name is `$prefix-$cluster-$namespace-$name`. And controller will manage the life cycle of the role, creating, modifying, and deleting the role.

```yaml
apiVersion: irsa.domc.me/v1alpha1
kind: IamRoleServiceAccount
metadata:
  name: iamroleserviceaccount-sample
spec:
  policy:
    managedPolicies:
      - arn:aws:iam::000000000000:policy/managedPolicy1
      - arn:aws:iam::000000000000:policy/managedPolicy2
    inlinePolicy:
      version: 2012-10-17
      statement:
        - effect: Allow
          resource:
            - "*"
          action:
            - "*"
```

## Configuration

| Parameter                 | Description                                                                                         | Required | Default |
| ------------------------- | --------------------------------------------------------------------------------------------------- | -------- | ------- |
| cluster                   | The name of the K8S cluster on which irsa-controller is running                                     | yes      |         |
| oidcProviderArn           | The oidc provider of the K8S cluster on which irsa-controller is running used to authenticate users | yes      |         |
| iamRolePrefix             | Prefix of the iam role name created by irsa-controller                                              | no       |         |
| awsConfig                 | AWS related configurations                                                                          | no       |         |
| awsConfig.endpoint        | The url of AWS IAM endpoint                                                                         | no       |         |
| awsConfig.accessKeyID     | The value of aws access key                                                                         | no       |         |
| awsConfig.secretAccessKey | The value of aws access key secret                                                                  | no       |         |
| awsConfig.disableSSL      | Whether disable SSL when connect to aws endpoint                                                    | no       |         |

## Permissions

The AWS permissions required by irsa-controller.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "VisualEditor0",
      "Effect": "Allow",
      "Action": [
        "iam:TagRole",
        "iam:CreateRole",
        "iam:DeleteRole",
        "iam:AttachRolePolicy",
        "iam:PutRolePolicy",
        "iam:DetachRolePolicy",
        "iam:DeleteRolePolicy",
        "iam:CreatePolicyVersion"
      ],
      "Resource": [
        "arn:aws:iam::$awsAccountId:role/$prefix-$cluster-*",
        "arn:aws:iam::$awsAccountId:policy/$prefix-$cluster-*"
      ]
    },
    {
      "Sid": "VisualEditor1",
      "Effect": "Allow",
      "Action": [
        "iam:UpdateAssumeRolePolicy",
        "iam:GetRole",
        "iam:ListAttachedRolePolicies",
        "iam:ListRolePolicies",
        "iam:GetRolePolicy"
      ],
      "Resource": "*"
    }
  ]
}
```
