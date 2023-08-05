package satellite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/IBM-Cloud/container-services-go-sdk/kubernetesserviceapiv1"
	"github.com/IBM-Cloud/terraform-provider-ibm/ibm/conns"
	"github.com/IBM-Cloud/terraform-provider-ibm/ibm/flex"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"k8s.io/utils/strings/slices"
)

func ResourceIBMSatelliteStorageConfiguration() *schema.Resource {
	return &schema.Resource{
		Create:   resourceIBMContainerStorageConfigurationCreate,
		Read:     resourceIBMContainerStorageConfigurationRead,
		Update:   resourceIBMContainerStorageConfigurationUpdate,
		Delete:   resourceIBMContainerStorageConfigurationDelete,
		Exists:   resourceIBMContainerStorageConfigurationExists,
		Importer: &schema.ResourceImporter{},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(20 * time.Minute),
			Update: schema.DefaultTimeout(20 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"location": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Location ID.",
			},
			"config_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "Name of the Storage Configuration.",
			},
			"config_version": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Version of the Storage Configuration.",
			},
			"storage_template_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The Storage Template Name.",
			},
			"storage_template_version": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The Storage Template Version.",
			},
			"user_config_parameters": {
				Type:     schema.TypeString,
				Required: true,
				StateFunc: func(v interface{}) string {
					json, err := flex.NormalizeJSONString(v)
					if err != nil {
						return fmt.Sprintf("%q", err.Error())
					}
					return json
				},
				Description: "User Config Parameters to pass in a JSON string format.",
			},
			"user_secret_parameters": {
				Type:     schema.TypeString,
				Required: true,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if len(d.Id()) == 0 {
						return false
					}
					var oldsecretParams map[string]string
					json.Unmarshal([]byte(old), &oldsecretParams)
					var newsecretParams map[string]string
					json.Unmarshal([]byte(new), &newsecretParams)
					for k, _ := range oldsecretParams {
						h := sha256.New()
						bs := h.Sum(nil)
						h.Write([]byte(newsecretParams[k]))
						securebs := hex.EncodeToString(bs)
						if !cmp.Equal(strings.Join([]string{"hash", "SHA3-256", securebs}, ":"), oldsecretParams[k]) {
							return false
						}
					}
					return true
				},
				StateFunc: func(v interface{}) string {
					json, err := flex.NormalizeJSONString(v)
					if err != nil {
						return fmt.Sprintf("%q", err.Error())
					}
					return json
				},
				Description: "User Secret Parameters to pass in a JSON string format.",
			},
			"storage_class_parameters": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type:    schema.TypeString,
					Default: "[]",
					StateFunc: func(v interface{}) string {
						json, err := flex.NormalizeJSONString(v)
						if err != nil {
							return fmt.Sprintf("%q", err.Error())
						}
						return json
					},
					Description: "The List of Storage Class Parameters.",
				},
			},
			"uuid": {
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
				Description: "The Universally Unique IDentifier (UUID) of the Storage Configuration.",
			},
		},
	}
}

func validateStorageConfig(d *schema.ResourceData, meta interface{}) error {
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	storageTemplateName := d.Get("storage_template_name").(string)
	storageTemplateVersion := d.Get("storage_template_version").(string)
	storageresult := &kubernetesserviceapiv1.GetStorageTemplateOptions{
		Name:    &storageTemplateName,
		Version: &storageTemplateVersion,
	}

	user_config_parameters := d.Get("user_config_parameters")
	var userconfigParams map[string]string
	json.Unmarshal([]byte(user_config_parameters.(string)), &userconfigParams)

	user_secret_parameters := d.Get("user_secret_parameters")
	var usersecretParams map[string]string
	json.Unmarshal([]byte(user_secret_parameters.(string)), &usersecretParams)

	result, _, err := satClient.GetStorageTemplate(storageresult)
	if err != nil {
		return err
	}

	var customparamList []string
	for _, v := range result.CustomParameters {
		var inInterface map[string]interface{}
		inrec, _ := json.Marshal(v)
		json.Unmarshal(inrec, &inInterface)
		if inInterface["required"].(string) == "true" {
			_, foundConfig := userconfigParams[inInterface["name"].(string)]
			_, foundSecret := usersecretParams[inInterface["name"].(string)]
			if !(foundConfig || foundSecret) {
				if len(inInterface["default"].(string)) != 0 {
					userconfigParams[inInterface["name"].(string)] = inInterface["default"].(string)
				} else {
					return fmt.Errorf("%s Parameter missing - Required", inInterface["name"].(string))
				}
			}
		}
		customparamList = append(customparamList, inInterface["name"].(string))
	}

	for k, _ := range userconfigParams {
		if !slices.Contains(customparamList, k) {
			return fmt.Errorf("Config Parameter %s not found", k)
		}
	}

	for k, _ := range usersecretParams {
		if !slices.Contains(customparamList, k) {
			return fmt.Errorf("Secret Parameter %s not found", k)
		}
	}

	return nil
}

func resourceIBMContainerStorageConfigurationCreate(d *schema.ResourceData, meta interface{}) error {
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	createStorageConfigurationOptions := &kubernetesserviceapiv1.CreateStorageConfigurationOptions{}
	satLocation := d.Get("location").(string)
	createStorageConfigurationOptions.Controller = &satLocation

	err = validateStorageConfig(d, meta)
	if err != nil {
		return err
	}

	if v, ok := d.GetOk("config_name"); ok {
		createStorageConfigurationOptions.SetConfigName(v.(string))
	}

	if v, ok := d.GetOk("storage_template_name"); ok {
		createStorageConfigurationOptions.SetStorageTemplateName(v.(string))
	}

	if v, ok := d.GetOk("storage_template_version"); ok {
		createStorageConfigurationOptions.SetStorageTemplateVersion(v.(string))
	}

	if v, ok := d.GetOk("user_config_parameters"); ok {
		var userconfigParams map[string]string
		json.Unmarshal([]byte(v.(string)), &userconfigParams)
		createStorageConfigurationOptions.SetUserConfigParameters(userconfigParams)
	}

	if v, ok := d.GetOk("user_secret_parameters"); ok {
		var usersecretParams map[string]string
		json.Unmarshal([]byte(v.(string)), &usersecretParams)
		createStorageConfigurationOptions.SetUserSecretParameters(usersecretParams)
	}

	if storageClassParamsList, ok := d.GetOk("storage_class_parameters"); ok {
		var mapString []map[string]string
		if len(storageClassParamsList.([]interface{})) != 0 {
			for _, value := range storageClassParamsList.([]interface{}) {
				var storageclassParams map[string]string
				json.Unmarshal([]byte(value.(string)), &storageclassParams)
				mapString = append(mapString, storageclassParams)
			}
			createStorageConfigurationOptions.SetStorageClassParameters(mapString)
		}
	}

	_, _, err = satClient.CreateStorageConfiguration(createStorageConfigurationOptions)
	if err != nil {
		return fmt.Errorf("Unable to Create Storage Configuration - %v", err)
	}
	getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
		Name: createStorageConfigurationOptions.ConfigName,
	}
	_, err = waitForStorageCreationStatus(getStorageConfigurationOptions, meta)
	if err != nil {
		return err
	}

	d.SetId(satLocation)
	return resourceIBMContainerStorageConfigurationRead(d, meta)
}

func resourceIBMContainerStorageConfigurationRead(d *schema.ResourceData, meta interface{}) error {

	var scDefinedList []map[string]string
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	satLocation := d.Get("location").(string)
	d.Set("location", satLocation)

	storageClassParamsList := d.Get("storage_class_parameters").([]interface{})
	if len(storageClassParamsList) != 0 {
		for _, value := range storageClassParamsList {
			var storageclassParams map[string]string
			json.Unmarshal([]byte(value.(string)), &storageclassParams)
			scDefinedList = append(scDefinedList, storageclassParams)
		}
	}

	storageConfigName := d.Get("config_name").(string)

	secretsMap := d.Get("user_secret_parameters")
	var usersecretParams map[string]string
	json.Unmarshal([]byte(secretsMap.(string)), &usersecretParams)

	getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
		Name: &storageConfigName,
	}

	result, _, err := satClient.GetStorageConfiguration(getStorageConfigurationOptions)
	if err != nil {
		return err
	}

	d.Set("config_name", *result.ConfigName)
	d.Set("config_version", *result.ConfigVersion)
	d.Set("storage_template_name", *result.StorageTemplateName)
	d.Set("storage_template_version", *result.StorageTemplateVersion)

	temp, _ := json.Marshal(result.UserConfigParameters)
	d.Set("user_config_parameters", string(temp))

	err = encodeSecretParameters(&usersecretParams, d)
	if err != nil {
		return err
	}
	temp, _ = json.Marshal(usersecretParams)
	d.Set("user_secret_parameters", string(temp))

	var storageClassList []string
	for _, v := range result.StorageClassParameters {
		if getDefinedStorageClasses(scDefinedList, v) {
			c, _ := json.Marshal(v)
			storageClassList = append(storageClassList, string(c))
		}
	}
	d.Set("storage_class_parameters", storageClassList)
	d.Set("uuid", *result.UUID)

	return nil

}

func resourceIBMContainerStorageConfigurationUpdate(d *schema.ResourceData, meta interface{}) error {
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	updateStorageConfigurationOptions := &kubernetesserviceapiv1.UpdateStorageConfigurationOptions{}
	satLocation := d.Get("location").(string)
	updateStorageConfigurationOptions.Controller = &satLocation

	err = validateStorageConfig(d, meta)
	if err != nil {
		return err
	}

	if d.HasChange("user_config_parameters") || d.HasChange("user_secret_parameters") || d.HasChange("storage_class_parameters") && !d.IsNewResource() {

		if v, ok := d.GetOk("config_name"); ok {
			updateStorageConfigurationOptions.SetConfigName(v.(string))
		}

		if v, ok := d.GetOk("storage_template_name"); ok {
			updateStorageConfigurationOptions.SetStorageTemplateName(v.(string))
		}

		if v, ok := d.GetOk("storage_template_version"); ok {
			updateStorageConfigurationOptions.SetStorageTemplateVersion(v.(string))
		}

		if v, ok := d.GetOk("user_config_parameters"); ok {
			var userconfigParams map[string]string
			json.Unmarshal([]byte(v.(string)), &userconfigParams)
			updateStorageConfigurationOptions.SetUserConfigParameters(userconfigParams)
		}

		if v, ok := d.GetOk("user_secret_parameters"); ok {
			var usersecretParams map[string]string
			json.Unmarshal([]byte(v.(string)), &usersecretParams)
			updateStorageConfigurationOptions.SetUserSecretParameters(usersecretParams)
		}

		if storageClassParamsList, ok := d.GetOk("storage_class_parameters"); ok {
			var mapStringList []map[string]string
			if len(storageClassParamsList.([]interface{})) != 0 {
				for _, value := range storageClassParamsList.([]interface{}) {
					var storageclassParams map[string]string
					json.Unmarshal([]byte(value.(string)), &storageclassParams)
					mapStringList = append(mapStringList, storageclassParams)
				}
				updateStorageConfigurationOptions.SetStorageClassParameters(mapStringList)
			}
		}

		_, _, err := satClient.UpdateStorageConfiguration(updateStorageConfigurationOptions)
		if err != nil {
			return fmt.Errorf("[ERROR] Unable to Update Storage Configuration %s - %v", *updateStorageConfigurationOptions.ConfigName, err)
		}

		getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
			Name: updateStorageConfigurationOptions.ConfigName,
		}
		_, err = waitForStorageCreationStatus(getStorageConfigurationOptions, meta)
		if err != nil {
			return err
		}
	}

	return resourceIBMContainerStorageConfigurationRead(d, meta)
}

func resourceIBMContainerStorageConfigurationDelete(d *schema.ResourceData, meta interface{}) error {
	uuid := d.Get("uuid").(string)
	name := d.Get("config_name").(string)
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	_, _, err = satClient.RemoveStorageConfiguration(&kubernetesserviceapiv1.RemoveStorageConfigurationOptions{
		UUID: &uuid,
	})
	if err != nil {
		return fmt.Errorf("[ERROR] Error Deleting Storage Configuration %s - %v", name, err)
	}
	getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
		Name: &name,
	}
	_, err = waitForStorageDeletionStatus(getStorageConfigurationOptions, meta)
	if err != nil {
		return err
	}
	d.SetId("")
	return nil
}

func getDefinedStorageClasses(definedMaps []map[string]string, getMaps map[string]string) bool {
	for _, v := range definedMaps {
		eq := reflect.DeepEqual(v, getMaps)
		if eq {
			return true
		}
	}
	return false
}

func encodeSecretParameters(secrets *map[string]string, d *schema.ResourceData) error {
	for k, v := range *secrets {
		if strings.Contains(k, "SHA3") {
			return nil
		}
		if len(d.Id()) == 0 {
			return fmt.Errorf("[ERROR] Resource Instance not Created")
		}
		h := sha256.New()
		bs := h.Sum(nil)
		h.Write([]byte(v))
		securebs := hex.EncodeToString(bs)
		(*secrets)[k] = strings.Join([]string{"hash", "SHA3-256", securebs}, ":")
	}
	return nil
}

func resourceIBMContainerStorageConfigurationExists(d *schema.ResourceData, meta interface{}) (bool, error) {
	return true, nil
}

func waitForStorageCreationStatus(getStorageConfigurationOptions *kubernetesserviceapiv1.GetStorageConfigurationOptions, meta interface{}) (interface{}, error) {
	stateConf := &resource.StateChangeConf{
		Pending:        []string{"NotReady"},
		Target:         []string{"Ready"},
		Refresh:        storageConfigurationStatusRefreshFunc(getStorageConfigurationOptions, meta),
		Timeout:        time.Duration(time.Minute * 10),
		Delay:          10 * time.Second,
		MinTimeout:     10 * time.Second,
		NotFoundChecks: 100,
	}
	return stateConf.WaitForState()
}

func storageConfigurationStatusRefreshFunc(getStorageConfigurationOptions *kubernetesserviceapiv1.GetStorageConfigurationOptions, meta interface{}) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {

		satClient, err := meta.(conns.ClientSession).SatelliteClientSession()

		if err != nil {
			return nil, "NotReady", err
		}
		_, response, err := satClient.GetStorageConfiguration(getStorageConfigurationOptions)

		if response.GetStatusCode() == 200 {
			return true, "Ready", nil
		}

		return nil, "NotReady", nil
	}
}

func waitForStorageDeletionStatus(getStorageConfigurationOptions *kubernetesserviceapiv1.GetStorageConfigurationOptions, meta interface{}) (interface{}, error) {
	stateConf := &resource.StateChangeConf{
		Pending:        []string{"NotReady"},
		Target:         []string{"Ready"},
		Refresh:        storageConfigurationDeletionStatusRefreshFunc(getStorageConfigurationOptions, meta),
		Timeout:        time.Duration(time.Minute * 10),
		Delay:          10 * time.Second,
		MinTimeout:     10 * time.Second,
		NotFoundChecks: 100,
	}
	return stateConf.WaitForState()
}

func storageConfigurationDeletionStatusRefreshFunc(getStorageConfigurationOptions *kubernetesserviceapiv1.GetStorageConfigurationOptions, meta interface{}) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {

		satClient, err := meta.(conns.ClientSession).SatelliteClientSession()

		if err != nil {
			return nil, "NotReady", err
		}
		_, response, err := satClient.GetStorageConfiguration(getStorageConfigurationOptions)
		if response.GetStatusCode() == 500 {
			return true, "Ready", nil
		}

		return nil, "NotReady", nil
	}
}
