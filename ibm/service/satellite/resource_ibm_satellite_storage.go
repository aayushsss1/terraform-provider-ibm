package satellite

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
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
		Create: resourceIBMContainerStorageConfigurationCreate,
		Read:   resourceIBMContainerStorageConfigurationRead,
		Update: resourceIBMContainerStorageConfigurationUpdate,
		Delete: resourceIBMContainerStorageConfigurationDelete,
		// Exists:   resourceIBMContainerStorageConfigurationExists,
		// Importer: &schema.ResourceImporter{},
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
			"UUID": {
				Type:        schema.TypeString,
				Computed:    true,
				ForceNew:    true,
				Description: "UUID.",
			},
			"storage_configuration": {
				Type:     schema.TypeSet,
				Required: true,
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
					},
				},
			},
		},
	}
}

func validateStorageConfig(createStorageConfigurationOptions *kubernetesserviceapiv1.CreateStorageConfigurationOptions, meta interface{}) error {

	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}
	storageTemplateName := createStorageConfigurationOptions.ConfigName
	storageTemplateVersion := createStorageConfigurationOptions.ConfigVersion
	storageresult := &kubernetesserviceapiv1.GetStorageTemplateOptions{
		Name:    storageTemplateName,
		Version: storageTemplateVersion,
	}
	userconfigParams := createStorageConfigurationOptions.UserConfigParameters
	usersecretParams := createStorageConfigurationOptions.UserSecretParameters
	result, _, _ := satClient.GetStorageTemplate(storageresult)
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

	log.Println("This is the customParamList", customparamList)

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

	log.Println("Validated Storage Configuration", createStorageConfigurationOptions)
	os.Exit(1)
	return nil
}

func resourceIBMContainerStorageConfigurationCreate(d *schema.ResourceData, meta interface{}) error {

	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	log.Println("In Create Function")
	createStorageConfigurationOptions := &kubernetesserviceapiv1.CreateStorageConfigurationOptions{}
	satLocation := d.Get("location").(string)
	createStorageConfigurationOptions.Location = &satLocation

	storageConfigSet := d.Get("storage_configuration").(*schema.Set).List()

	for _, scSet := range storageConfigSet {
		sc, _ := scSet.(map[string]interface{})

		configName := sc["config_name"].(string)
		createStorageConfigurationOptions.ConfigName = &configName

		storageTemplateName := sc["storage_template_name"].(string)
		createStorageConfigurationOptions.StorageTemplateName = &storageTemplateName

		storageTemplateVersion := sc["storage_template_version"].(string)
		createStorageConfigurationOptions.StorageTemplateVersion = &storageTemplateVersion

		var userconfigParams map[string]string
		json.Unmarshal([]byte(sc["user_config_parameters"].(string)), &userconfigParams)
		createStorageConfigurationOptions.UserConfigParameters = userconfigParams

		if sc["user_secret_parameters"] != "" {
			var usersecretParams map[string]string
			json.Unmarshal([]byte(sc["user_secret_parameters"].(string)), &usersecretParams)
			createStorageConfigurationOptions.UserSecretParameters = usersecretParams
		}

		storageClassParamsList := sc["storage_class_parameters"].([]interface{})
		var mapString []map[string]string
		if len(storageClassParamsList) != 0 {
			for _, value := range storageClassParamsList {
				var storageclassParams map[string]string
				json.Unmarshal([]byte(value.(string)), &storageclassParams)
				log.Println(storageclassParams)
				mapString = append(mapString, storageclassParams)
			}
			createStorageConfigurationOptions.StorageClassParameters = mapString
		}

		err = validateStorageConfig(createStorageConfigurationOptions, meta)
		if err != nil {
			return err
		}

		result, _, err := satClient.CreateStorageConfiguration(createStorageConfigurationOptions)

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

		d.Set("UUID", result.AddChannel.UUID)

	}

	return nil

}

func resourceIBMContainerStorageConfigurationRead(d *schema.ResourceData, meta interface{}) error {

	return nil

}

func resourceIBMContainerStorageConfigurationUpdate(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceIBMContainerStorageConfigurationDelete(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceIBMContainerStorageConfigurationExists(d *schema.ResourceData, meta interface{}) error {
	return nil
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
