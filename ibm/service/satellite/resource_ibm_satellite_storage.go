package satellite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
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
				Description: "Location ID.",
			},
			"storage_configuration": {
				Type:        schema.TypeList,
				Required:    true,
				Description: "The Set of different Storage Configurations",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"config_name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "Name of the Storage Configuration.",
						},
						"storage_template_name": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The Storage Template Name.",
						},
						"storage_template_version": {
							Type:        schema.TypeString,
							Required:    true,
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
				},
			},
		},
	}
}

func validateStorageConfig(sc map[string]interface{}, meta interface{}) error {
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	storageTemplateName := sc["storage_template_name"].(string)
	storageTemplateVersion := sc["storage_template_version"].(string)
	storageresult := &kubernetesserviceapiv1.GetStorageTemplateOptions{
		Name:    &storageTemplateName,
		Version: &storageTemplateVersion,
	}

	var userconfigParams map[string]string
	json.Unmarshal([]byte(sc["user_config_parameters"].(string)), &userconfigParams)

	var usersecretParams map[string]string
	json.Unmarshal([]byte(sc["user_secret_parameters"].(string)), &usersecretParams)

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
	storageConfigSet := d.Get("storage_configuration").([]interface{})

	for _, scSet := range storageConfigSet {
		sc, _ := scSet.(map[string]interface{})

		err = validateStorageConfig(sc, meta)
		if err != nil {
			return err
		}

		if v, ok := sc["config_name"]; ok {
			createStorageConfigurationOptions.SetConfigName(v.(string))
		}

		if v, ok := sc["storage_template_name"]; ok {
			createStorageConfigurationOptions.SetStorageTemplateName(v.(string))
		}

		if v, ok := sc["storage_template_version"]; ok {
			createStorageConfigurationOptions.SetStorageTemplateVersion(v.(string))
		}

		if v, ok := sc["user_config_parameters"]; ok {
			var userconfigParams map[string]string
			json.Unmarshal([]byte(v.(string)), &userconfigParams)
			createStorageConfigurationOptions.SetUserConfigParameters(userconfigParams)
		}

		if v, ok := sc["user_secret_parameters"]; ok {
			var usersecretParams map[string]string
			json.Unmarshal([]byte(v.(string)), &usersecretParams)
			createStorageConfigurationOptions.SetUserSecretParameters(usersecretParams)
		}

		if storageClassParamsList, ok := sc["storage_class_parameters"].([]interface{}); ok {
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
	var mapString = make(map[string][]map[string]string)
	var userSecretParameters = make(map[string](map[string]string))
	var storageConfigList []string
	storageConfigurations := []interface{}{}

	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	storageConfigSet := d.Get("storage_configuration").([]interface{})
	satLocation := d.Get("location").(string)
	d.Set("location", satLocation)
	for _, scSet := range storageConfigSet {
		sc, _ := scSet.(map[string]interface{})
		// List of Storage Configuration Names
		storageConfigList = append(storageConfigList, sc["config_name"].(string))

		// Map of Storage Configuration Name and the corresponding Storage Class Parameter List
		storageClassParamsList := sc["storage_class_parameters"].([]interface{})
		if len(storageClassParamsList) != 0 {
			for _, value := range storageClassParamsList {
				var storageclassParams map[string]string
				json.Unmarshal([]byte(value.(string)), &storageclassParams)
				mapString[sc["config_name"].(string)] = append(mapString[sc["config_name"].(string)], storageclassParams)
			}
		}
		// Map of Storage Configuration Name and the corresponding User Secrets Parameters Map
		var usersecretParams map[string]string
		json.Unmarshal([]byte(sc["user_secret_parameters"].(string)), &usersecretParams)
		userSecretParameters[sc["config_name"].(string)] = usersecretParams
	}

	// Iterate through all the Storage Names and Set the respective parameters
	for _, storageconfigname := range storageConfigList {

		getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
			Name: &storageconfigname,
		}

		result, _, err := satClient.GetStorageConfiguration(getStorageConfigurationOptions)
		if err != nil {
			return err
		}
		// Set the records with results from the server
		record := map[string]interface{}{}
		record["config_name"] = *result.ConfigName
		record["storage_template_name"] = *result.StorageTemplateName
		record["storage_template_version"] = *result.StorageTemplateVersion
		temp, _ := json.Marshal(result.UserConfigParameters)
		record["user_config_parameters"] = string(temp)
		// Secrets are encoded from the server, a local hashed copy is saved in the tfstate file, to allow api recycling
		secretMap := userSecretParameters[*result.ConfigName]
		// sha256 encoding
		err = encodeSecretParameters(&secretMap, d)
		if err != nil {
			return err
		}
		temp, _ = json.Marshal(secretMap)
		record["user_secret_parameters"] = string(temp)
		// Only the Storage Classes Defined in the Terraform Template is added to the tfstate file this prevents irrelevant differences
		var storageClassList []string
		for _, v := range result.StorageClassParameters {
			if getDefinedStorageClasses(mapString[record["config_name"].(string)], v) {
				c, _ := json.Marshal(v)
				storageClassList = append(storageClassList, string(c))
			}
		}
		record["storage_class_parameters"] = storageClassList
		record["uuid"] = *result.UUID
		storageConfigurations = append(storageConfigurations, record)
	}
	// Set the Storage Configuration
	d.Set("storage_configuration", storageConfigurations)
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

	if d.HasChange("storage_configuration") && !d.IsNewResource() {
		_, newList := d.GetChange("storage_configuration")
		ns := newList.([]interface{})
		for _, scSet := range ns {
			sc, _ := scSet.(map[string]interface{})

			err = validateStorageConfig(sc, meta)
			if err != nil {
				return err
			}

			if v, ok := sc["config_name"]; ok {
				updateStorageConfigurationOptions.SetConfigName(v.(string))
			}

			if v, ok := sc["storage_template_name"]; ok {
				updateStorageConfigurationOptions.SetStorageTemplateName(v.(string))
			}

			if v, ok := sc["storage_template_version"]; ok {
				updateStorageConfigurationOptions.SetStorageTemplateVersion(v.(string))
			}

			if v, ok := sc["user_config_parameters"]; ok {
				var userconfigParams map[string]string
				json.Unmarshal([]byte(v.(string)), &userconfigParams)
				updateStorageConfigurationOptions.SetUserConfigParameters(userconfigParams)
			}

			if v, ok := sc["user_secret_parameters"]; ok {
				var usersecretParams map[string]string
				json.Unmarshal([]byte(v.(string)), &usersecretParams)
				updateStorageConfigurationOptions.SetUserSecretParameters(usersecretParams)
			}

			if storageClassParamsList, ok := sc["storage_class_parameters"].([]interface{}); ok {
				var mapString []map[string]string
				if len(storageClassParamsList) != 0 {
					for _, value := range storageClassParamsList {
						var storageclassParams map[string]string
						json.Unmarshal([]byte(value.(string)), &storageclassParams)
						log.Println(storageclassParams)
						mapString = append(mapString, storageclassParams)
					}
					updateStorageConfigurationOptions.SetStorageClassParameters(mapString)
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
	}

	return resourceIBMContainerStorageConfigurationRead(d, meta)
}

func resourceIBMContainerStorageConfigurationDelete(d *schema.ResourceData, meta interface{}) error {
	storageConfigSet := d.Get("storage_configuration").([]interface{})
	uuidMap := make(map[string]string)
	for _, scSet := range storageConfigSet {
		sc, _ := scSet.(map[string]interface{})
		uuidMap[sc["config_name"].(string)] = sc["uuid"].(string)
	}
	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	for k, v := range uuidMap {
		_, _, err := satClient.RemoveStorageConfiguration(&kubernetesserviceapiv1.RemoveStorageConfigurationOptions{
			UUID: &v,
		})
		if err != nil {
			return fmt.Errorf("[ERROR] Error Deleting Storage Configuration %s - %v", k, err)
		}
		getStorageConfigurationOptions := &kubernetesserviceapiv1.GetStorageConfigurationOptions{
			Name: &k,
		}
		_, err = waitForStorageDeletionStatus(getStorageConfigurationOptions, meta)
		if err != nil {
			return err
		}
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
