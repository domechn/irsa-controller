# irsa-controller

[![Docker](https://github.com/domechn/irsa-controller/actions/workflows/build-and-publish.yaml/badge.svg)](https://github.com/domechn/irsa-controller/actions/workflows/build-and-publish.yaml)

Using CRD to manage Kubernetes `ServiceAccount` and AWS `Iam Role`.

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

```yaml
apiVersion: irsa.domc.me/v1alpha1
kind: IamRoleServiceAccount
metadata:
  name: iamroleserviceaccount-sample
spec:
  roleName: <external-iam-role-name>
```

### Use CRD to define permissions for iam role

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
