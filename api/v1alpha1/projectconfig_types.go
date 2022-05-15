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

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cfg "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProjectConfigSpec defines the desired state of ProjectConfig
type ProjectConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	IamRolePrefix   string         `json:"iamRolePrefix,omitempty"`
	OIDCProviderArn string         `json:"oidcProviderArn,omitempty"`
	AdditionalTags  []string       `json:"additionalTags,omitempty"`
	AWSConfig       *AWSConfigSpec `json:"awsConfig,omitempty"`
}

type AWSConfigSpec struct {
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	AccessKeyID     string `json:"accessKeyID,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	DisableSSL      bool   `json:"disableSSL,omitempty"`
}

// ProjectConfigStatus defines the observed state of ProjectConfig
type ProjectConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProjectConfig is the Schema for the projectconfigs API
type ProjectConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	ProjectConfigSpec `json:",inline"`
	Status            ProjectConfigStatus `json:"status,omitempty"`

	// ControllerManagerConfigurationSpec returns the contfigurations for controllers
	cfg.ControllerManagerConfigurationSpec `json:",inline"`
}

//+kubebuilder:object:root=true

// ProjectConfigList contains a list of ProjectConfig
type ProjectConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProjectConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProjectConfig{}, &ProjectConfigList{})
}

func (p *ProjectConfig) Validate() error {
	if p.OIDCProviderArn == "" {
		return fmt.Errorf("OIDCProviderArn is required.")
	}

	if p.ClusterName == "" {
		return fmt.Errorf("ClusterName is required.")
	}

	if p.AWSConfig != nil {
		if p.AWSConfig.AccessKeyID == "" || p.AWSConfig.SecretAccessKey == "" {
			return fmt.Errorf("AccessKeyID and SecretAccessKey is required when aws config is setten.")
		}
	}
	return nil
}
