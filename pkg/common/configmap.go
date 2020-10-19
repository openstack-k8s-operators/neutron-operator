package common

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	util "github.com/openstack-k8s-operators/lib-common/pkg/util"
	neutronv1beta1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CMType - configmap type
type CMType string

const (
	// CMTypeScripts - config CM type
	CMTypeScripts CMType = "bin"
	// CMTypeConfig - config CM type
	CMTypeConfig CMType = "config"
	// CMTypeCustom - custom config CM type
	CMTypeCustom CMType = "custom"
)

// ConfigMap - config map details
type ConfigMap struct {
	Name           string
	Namespace      string
	CMType         CMType
	InstanceType   string
	AdditionalData map[string]string
	Labels         map[string]string
	ConfigOptions  map[string]string
}

// GetConfigMaps - get all configmaps required, verify they exist and add the hash to env and status
func GetConfigMaps(r ReconcilerCommon, obj runtime.Object, configMaps []string, namespace string, envVars *map[string]util.EnvSetter, configPrefix string) ([]neutronv1beta1.Hash, error) {
	hashes := []neutronv1beta1.Hash{}

	for _, cm := range configMaps {
		_, hash, err := GetConfigMapAndHashWithName(r, cm, namespace)
		if err != nil {
			return nil, err
		}
		(*envVars)[cm] = util.EnvValue(hash)
		hashes = append(hashes, neutronv1beta1.Hash{Name: cm, Hash: hash})
	}

	return hashes, nil
}

// CreateOrUpdateConfigMap -
func CreateOrUpdateConfigMap(c client.Client, log logr.Logger, configMap *corev1.ConfigMap) error {
	// Check if this configMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
		err = c.Create(context.TODO(), configMap)
		if err != nil {
			return err
		}
	} else if !reflect.DeepEqual(configMap.Data, foundConfigMap.Data) {
		log.Info(fmt.Sprintf("Updating ConfigMap: %s", configMap.Name))
		foundConfigMap.Data = configMap.Data
		err = c.Update(context.TODO(), foundConfigMap)
		if err != nil {
			return err
		}
	}
	return nil
}

// CreateOrGetCustomConfigMap -
func CreateOrGetCustomConfigMap(c client.Client, log logr.Logger, configMap *corev1.ConfigMap) error {
	// Check if this configMap already exists
	foundConfigMap := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
		err = c.Create(context.TODO(), configMap)
		if err != nil {
			return err
		}
	} else {
		// use data from already existing custom configmap
		configMap.Data = foundConfigMap.Data
	}
	return nil
}

// GetConfigMapAndHashWithName -
func GetConfigMapAndHashWithName(r ReconcilerCommon, configMapName string, namespace string) (*corev1.ConfigMap, string, error) {

	configMap := &corev1.ConfigMap{}
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: configMapName, Namespace: namespace}, configMap)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Error(err, configMapName+" ConfigMap not found!", "Instance.Namespace", namespace, "ConfigMap.Name", configMapName)
		return configMap, "", err
	}
	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return configMap, "", fmt.Errorf("error calculating configuration hash: %v", err)
	}
	return configMap, configMapHash, nil
}

// EnsureConfigMaps - get all configmaps required, verify they exist and add the hash to env and status
func EnsureConfigMaps(r ReconcilerCommon, obj metav1.Object, cms []ConfigMap, envVars *map[string]util.EnvSetter) error {
	var err error

	for _, cm := range cms {
		var hash string
		var op controllerutil.OperationResult

		if cm.CMType != CMTypeCustom {
			hash, op, err = createOrUpdateConfigMap(r, obj, cm)
		} else {
			hash, err = createOrGetCustomConfigMap(r, obj, cm)
		}
		if err != nil {
			return err
		}
		if op != controllerutil.OperationResultNone {
			r.GetLogger().Info(fmt.Sprintf("ConfigMap %s successfully reconciled - operation: %s", cm.Name, string(op)))
			return nil
		}
		if envVars != nil {
			(*envVars)[cm.Name] = util.EnvValue(hash)
		}
	}

	return nil
}

// createOrUpdateConfigMap -
func createOrUpdateConfigMap(r ReconcilerCommon, obj metav1.Object, cm ConfigMap) (string, controllerutil.OperationResult, error) {
	data := make(map[string]string)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.Name,
			Namespace: cm.Namespace,
		},
		Data: data,
	}

	// create or update the CM
	op, err := controllerutil.CreateOrUpdate(context.TODO(), r.GetClient(), configMap, func() error {

		configMap.Labels = cm.Labels
		// add data from templates
		configMap.Data = getConfigMapData(cm)

		err := controllerutil.SetControllerReference(obj, configMap, r.GetScheme())
		if err != nil {
			return err
		}

		return nil
	})

	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return "", op, fmt.Errorf("error calculating configuration hash: %v", err)
	}

	return configMapHash, op, nil
}

// getConfigMapData -
func getConfigMapData(cm ConfigMap) map[string]string {
	//opts := ConfigOptions{}
	opts := cm.ConfigOptions

	// get templates base path, either running local or deployed as container
	templatesPath := util.GetTemplatesPath()

	// get all scripts templates which are in ../templesPath/cr.Kind/CMType
	templatesFiles := util.GetAllTemplates(templatesPath, cm.InstanceType, string(cm.CMType))

	data := make(map[string]string)
	// render all template files
	for _, file := range templatesFiles {
		data[filepath.Base(file)] = util.ExecuteTemplate(file, opts)
	}
	// add additional files e.g. from different directory, which are common
	// to multiple controllers
	for filename, file := range cm.AdditionalData {
		data[filename] = util.ExecuteTemplateFile(file, opts)
	}

	return data
}

// createOrGetCustomConfigMap -
func createOrGetCustomConfigMap(r ReconcilerCommon, obj metav1.Object, cm ConfigMap) (string, error) {
	// Check if this configMap already exists
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Labels:    cm.Labels,
		},
		Data: map[string]string{},
	}
	foundConfigMap := &corev1.ConfigMap{}
	err := r.GetClient().Get(context.TODO(), types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, foundConfigMap)
	if err != nil && k8s_errors.IsNotFound(err) {
		err := controllerutil.SetControllerReference(obj, configMap, r.GetScheme())
		if err != nil {
			return "", err
		}

		r.GetLogger().Info(fmt.Sprintf("Creating a new ConfigMap %s in namespace %s", cm.Namespace, cm.Name))
		err = r.GetClient().Create(context.TODO(), configMap)
		if err != nil {
			return "", err
		}
	} else {
		// use data from already existing custom configmap
		configMap.Data = foundConfigMap.Data
	}

	configMapHash, err := util.ObjectHash(configMap)
	if err != nil {
		return "", fmt.Errorf("error calculating configuration hash: %v", err)
	}

	return configMapHash, nil
}
