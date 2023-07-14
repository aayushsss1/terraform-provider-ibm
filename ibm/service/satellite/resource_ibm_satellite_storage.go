package satellite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/IBM-Cloud/container-services-go-sdk/kubernetesserviceapiv1"
	"github.com/IBM-Cloud/terraform-provider-ibm/ibm/conns"
	"github.com/IBM-Cloud/terraform-provider-ibm/ibm/flex"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"k8s.io/utils/strings/slices"
)

func ResourceIBMSatelliteStorage() *schema.Resource {
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
				ForceNew:    true,
				Description: "Location ID.",
			},
			"storage_configuration": {
				Type:     schema.TypeSet,
				Required: true,
				Set:      resourceIBMContainerAddonsHash,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"config_name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Name of your Storage Configuration",
						},
						"storage_template_name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The Storage Template Name",
						},
						"storage_template_version": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The storage template version",
						},
						"user_config_parameters": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "",
							StateFunc: func(v interface{}) string {
								json, err := flex.NormalizeJSONString(v)
								if err != nil {
									return fmt.Sprintf("%q", err.Error())
								}
								return json
							},
							Description: "user config parameters to pass in a JSON string format.",
						},
						"user_secret_parameters": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  "",
							StateFunc: func(v interface{}) string {
								json, err := flex.NormalizeJSONString(v)
								if err != nil {
									return fmt.Sprintf("%q", err.Error())
								}
								return json
							},
							Description: "user config parameters to pass in a JSON string format.",
						},
						"storage_class_parameters": {
							Type:     schema.TypeList,
							Optional: true,
							Elem: &schema.Schema{
								Type:    schema.TypeString,
								Default: "",
								StateFunc: func(v interface{}) string {
									json, err := flex.NormalizeJSONString(v)
									if err != nil {
										return fmt.Sprintf("%q", err.Error())
									}
									return json
								},
								Description: "A list of Storage Class Parameters",
							},
						},
						"uuid": {
							Type:        schema.TypeString,
							Computed:    true,
							ForceNew:    true,
							Description: "UUID",
						},
					},
				},
			},
		},
	}
}

type configValidation interface {
	// Methods
	validateCreateStorageConfig(createStorageConfigurationOptions *kubernetesserviceapiv1.CreateStorageConfigurationOptions, meta interface{}) error
	validateUpdateStorageConfig(updateStorageConfigurationOptions *kubernetesserviceapiv1.UpdateStorageConfigurationOptions, meta interface{}) error
}

type validateCreation struct{}
type validateUpdate struct{}

func (create validateCreation) validateCreateStorageConfig(createStorageConfigurationOptions *kubernetesserviceapiv1.CreateStorageConfigurationOptions, meta interface{}) error {
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	storageTemplateName := createStorageConfigurationOptions.StorageTemplateName
	storageTemplateVersion := createStorageConfigurationOptions.StorageTemplateVersion
	storageresult := &kubernetesserviceapiv1.GetStorageTemplateOptions{}
	storageresult.SetName(*storageTemplateName)
	storageresult.SetVersion(*storageTemplateVersion)
	userconfigParams := createStorageConfigurationOptions.UserConfigParameters
	usersecretParams := createStorageConfigurationOptions.UserSecretParameters
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
			if !(foundConfig || foundSecret) && len(inInterface["default"].(string)) != 0 {
				userconfigParams[inInterface["name"].(string)] = inInterface["default"].(string)
			} else {
				return fmt.Errorf("%s Parameter missing", inInterface["name"].(string))
			}
		}
		customparamList = append(customparamList, inInterface["name"].(string))
	}

	for k, _ := range userconfigParams {
		if !slices.Contains(customparamList, k) {
			return fmt.Errorf("Parameter %s not found", k)
		}
	}

	for k, _ := range usersecretParams {
		if !slices.Contains(customparamList, k) {
			return fmt.Errorf("Parameter %s not found", k)
		}
	}

	return nil
}

func (update validateUpdate) validateUpdateStorageConfig(updateStorageConfigurationOptions *kubernetesserviceapiv1.UpdateStorageConfigurationOptions, meta interface{}) error {
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	storageTemplateName := updateStorageConfigurationOptions.StorageTemplateName
	storageTemplateVersion := updateStorageConfigurationOptions.StorageTemplateVersion
	storageresult := &kubernetesserviceapiv1.GetStorageTemplateOptions{}
	storageresult.SetName(*storageTemplateName)
	storageresult.SetVersion(*storageTemplateVersion)
	userconfigParams := updateStorageConfigurationOptions.UserConfigParameters
	usersecretParams := updateStorageConfigurationOptions.UserSecretParameters
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
				return fmt.Errorf("%s Parameter missing", inInterface["name"].(string))
			}
		}
		customparamList = append(customparamList, inInterface["name"].(string))
	}

	for k, _ := range userconfigParams {
		if !slices.Contains(customparamList, k) {
			return fmt.Errorf("Parameter %s not found", k)
		}
	}

	for k, _ := range usersecretParams {
		if !slices.Contains(customparamList, k) {
			return fmt.Errorf("Parameter %s not found", k)
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
	storageConfigSet := d.Get("storage_configuration").(*schema.Set).List()
	t := validateCreation{}
	for _, scSet := range storageConfigSet {
		sc, _ := scSet.(map[string]interface{})
		createStorageConfigurationOptions.SetConfigName(sc["config_name"].(string))
		createStorageConfigurationOptions.SetStorageTemplateName(sc["storage_template_name"].(string))
		createStorageConfigurationOptions.SetStorageTemplateVersion(sc["storage_template_version"].(string))

		var userconfigParams map[string]string
		json.Unmarshal([]byte(sc["user_config_parameters"].(string)), &userconfigParams)
		createStorageConfigurationOptions.SetUserConfigParameters(userconfigParams)

		var usersecretParams map[string]string
		json.Unmarshal([]byte(sc["user_secret_parameters"].(string)), &usersecretParams)
		createStorageConfigurationOptions.SetUserSecretParameters(usersecretParams)

		storageClassParamsList := sc["storage_class_parameters"].([]interface{})
		var mapString []map[string]string
		if len(storageClassParamsList) != 0 {
			for _, value := range storageClassParamsList {
				var storageclassParams map[string]string
				json.Unmarshal([]byte(value.(string)), &storageclassParams)
				log.Println(storageclassParams)
				mapString = append(mapString, storageclassParams)
			}
			createStorageConfigurationOptions.SetStorageClassParameters(mapString)
		}

		err = t.validateCreateStorageConfig(createStorageConfigurationOptions, meta)
		if err != nil {
			return err
		}

		_, _, err := satClient.CreateStorageConfiguration(createStorageConfigurationOptions)
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
	}
	d.SetId(satLocation)
	return resourceIBMContainerStorageConfigurationRead(d, meta)
}

func resourceIBMContainerStorageConfigurationRead(d *schema.ResourceData, meta interface{}) error {

	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	storageConfigSet := d.Get("storage_configuration").(*schema.Set).List()
	var storageConfigList []string
	storageConfigurations := []interface{}{}
	for _, scSet := range storageConfigSet {
		sc, _ := scSet.(map[string]interface{})
		storageConfigList = append(storageConfigList, sc["config_name"].(string))
	}

	satLocation := d.Get("location").(string)
	d.Set("location", satLocation)

	for _, storageconfigname := range storageConfigList {
		getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
			Name: &storageconfigname,
		}
		result, _, err := satClient.GetStorageConfiguration(getStorageConfigurationOptions)
		if err != nil {
			return err
		}
		record := map[string]interface{}{}
		record["config_name"] = *result.ConfigName
		record["storage_template_name"] = *result.StorageTemplateName
		record["storage_template_version"] = *result.StorageTemplateVersion
		a, _ := json.Marshal(result.UserConfigParameters)
		record["user_config_parameters"] = string(a)
		b, _ := json.Marshal(result.UserSecretParameters)
		record["user_secret_parameters"] = string(b)
		var storageClassList []string
		for _, v := range result.StorageClassParameters {
			c, _ := json.Marshal(v)
			storageClassList = append(storageClassList, string(c))
		}
		record["storage_class_parameters"] = storageClassList
		record["uuid"] = *result.UUID
		storageConfigurations = append(storageConfigurations, record)
	}
	storageConfigurationsSet := schema.NewSet(resourceIBMContainerAddonsHash, storageConfigurations)
	d.Set("storage_configuration", storageConfigurationsSet)

	return nil
}

func resourceIBMContainerStorageConfigurationUpdate(d *schema.ResourceData, meta interface{}) error {

	t := validateUpdate{}
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	updateStorageConfigurationOptions := &kubernetesserviceapiv1.UpdateStorageConfigurationOptions{}
	satLocation := d.Get("location").(string)
	updateStorageConfigurationOptions.Controller = &satLocation

	if d.HasChange("storage_configuration") && !d.IsNewResource() {
		_, newList := d.GetChange("storage_configuration")
		ns := newList.(*schema.Set).List()
		for _, scSet := range ns {
			sc, _ := scSet.(map[string]interface{})
			updateStorageConfigurationOptions.SetConfigName(sc["config_name"].(string))
			updateStorageConfigurationOptions.SetStorageTemplateName(sc["storage_template_name"].(string))
			updateStorageConfigurationOptions.SetStorageTemplateVersion(sc["storage_template_version"].(string))

			var userconfigParams map[string]string
			json.Unmarshal([]byte(sc["user_config_parameters"].(string)), &userconfigParams)
			updateStorageConfigurationOptions.SetUserConfigParameters(userconfigParams)

			var usersecretParams map[string]string
			json.Unmarshal([]byte(sc["user_secret_parameters"].(string)), &usersecretParams)
			updateStorageConfigurationOptions.SetUserSecretParameters(usersecretParams)

			storageClassParamsList := sc["storage_class_parameters"].([]interface{})
			var mapString []map[string]string
			if len(storageClassParamsList) != 0 {
				for _, value := range storageClassParamsList {
					var storageclassParams map[string]string
					json.Unmarshal([]byte(value.(string)), &storageclassParams)
					mapString = append(mapString, storageclassParams)
				}
				updateStorageConfigurationOptions.SetStorageClassParameters(mapString)
			}
			err = t.validateUpdateStorageConfig(updateStorageConfigurationOptions, meta)
			if err != nil {
				return err
			}
			_, _, err := satClient.UpdateStorageConfiguration(updateStorageConfigurationOptions)
			if err != nil {
				return fmt.Errorf("Unable to Update Storage Configuration - %v", err)
			}
			getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
				Name: updateStorageConfigurationOptions.ConfigName,
			}
			_, err = waitForStorageCreationStatus(getStorageConfigurationOptions, meta)
			if err != nil {
				return err
			}
		}
	}

	return resourceIBMContainerStorageConfigurationRead(d, meta)
}

func resourceIBMContainerStorageConfigurationDelete(d *schema.ResourceData, meta interface{}) error {
	storageConfigSet := d.Get("storage_configuration").(*schema.Set).List()
	var uuidList []string
	for _, scSet := range storageConfigSet {
		sc, _ := scSet.(map[string]interface{})
		uuidList = append(uuidList, sc["uuid"].(string))
	}
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	for _, v := range uuidList {
		_, _, err = satClient.RemoveStorageConfiguration(&kubernetesserviceapiv1.RemoveStorageConfigurationOptions{
			UUID: &v,
		})
		if err != nil {
			return fmt.Errorf("[ERROR] Deleting Storage Config with UUID %s - %v ", v, err)
		}
	}
	d.SetId("")
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

func resourceIBMContainerAddonsHash(v interface{}) int {
	var buf bytes.Buffer
	a := v.(map[string]interface{})
	log.Println("This is the value of 'a' ", a)
	buf.WriteString(fmt.Sprintf("%s-", a["storage_template_name"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", a["storage_template_version"].(string)))

	return conns.String(buf.String())
}

/*

upgrade scenario ->
1. If change is present in storage_configuration
	- Get New List
	- Validate
	- Send an API
	- Wait for update

2. Delete
*/

// Test commit
