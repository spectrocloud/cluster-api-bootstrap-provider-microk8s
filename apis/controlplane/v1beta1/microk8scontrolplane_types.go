/*
Copyright 2022.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MicroK8sControlPlaneSpec defines the desired state of MicroK8sControlPlane
type MicroK8sControlPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of MicroK8sControlPlane. Edit microk8scontrolplane_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// MicroK8sControlPlaneStatus defines the observed state of MicroK8sControlPlane
type MicroK8sControlPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MicroK8sControlPlane is the Schema for the microk8scontrolplanes API
type MicroK8sControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MicroK8sControlPlaneSpec   `json:"spec,omitempty"`
	Status MicroK8sControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MicroK8sControlPlaneList contains a list of MicroK8sControlPlane
type MicroK8sControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MicroK8sControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MicroK8sControlPlane{}, &MicroK8sControlPlaneList{})
}
