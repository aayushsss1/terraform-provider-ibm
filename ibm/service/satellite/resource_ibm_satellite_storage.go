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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
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

func validateStorageConfig(storageConfigSet []interface{}) bool {
	return true
}

func resourceIBMContainerStorageConfigurationCreate(d *schema.ResourceData, meta interface{}) error {

	satClient, err := meta.(conns.ClientSession).SatelliteClientSession()
	if err != nil {
		return err
	}

	name := "odf-remote"
	version := "4.11"

	storageresult := &kubernetesserviceapiv1.GetStorageTemplateOptions{
		Name:    &name,
		Version: &version,
	}
	result, _, _ := satClient.GetStorageTemplate(storageresult)
	log.Println("This is the result")
	log.Println()
	// m := structs.Map(result)
	log.Println("This is the custom Parameters", result.CustomParameters[0])
	log.Println("This is the Description", *result.Description)
	log.Println("This is the Name", *result.Name)
	var inInterface map[string]interface{}
	inrec, _ := json.Marshal(result.CustomParameters[0])
	json.Unmarshal(inrec, &inInterface)

	// iterate through inrecs
	for field, val := range inInterface {
		fmt.Println("KV Pair: ", field, val)
	}

	log.Println(*result)

	os.Exit(1)

	log.Println("In Create Function")
	createStorageConfigurationOptions := &kubernetesserviceapiv1.CreateStorageConfigurationOptions{}
	satLocation := d.Get("location").(string)
	createStorageConfigurationOptions.Location = &satLocation

	storageConfigSet := d.Get("storage_configuration").(*schema.Set).List()
	if !validateStorageConfig(storageConfigSet) {
		return fmt.Errorf("Incorrect Parameters Provided")
	}

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
		if len(storageClassParamsList) != 0 {
			for _, value := range storageClassParamsList {
				var storageclassParams map[string]string
				json.Unmarshal([]byte(value.(string)), &storageclassParams)
				log.Println(storageclassParams)
			}
		}
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
