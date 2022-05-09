# irsa-controller

Using CRD to manage Kubernetes `ServiceAccount` and AWS `Iam Role`.

## Installation

### From Source Code

```shell
git clone https://github.com/domechn/irsa-controller
cd irsa-controller
make deploy
```

### Helm Chart

TODO

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
            - '*'
          action:
            - '*'
```
