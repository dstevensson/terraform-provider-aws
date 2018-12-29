package aws

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/globalaccelerator"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
)

func resourceAwsGlobalAcceleratorAccelerator() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsGlobalAcceleratorAcceleratorCreate,
		Read:   resourceAwsGlobalAcceleratorAcceleratorRead,
		Update: resourceAwsGlobalAcceleratorAcceleratorUpdate,
		Delete: resourceAwsGlobalAcceleratorAcceleratorDelete,

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(5 * time.Minute),
			Update: schema.DefaultTimeout(5 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"ip_address_type": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ValidateFunc: validation.StringInSlice([]string{
					globalaccelerator.IpAddressTypeIpv4,
				}, false),
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},
			"ip_sets": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"ip_addresses": {
							Type:     schema.TypeList,
							Computed: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"ip_family": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"attributes": {
				Type:     schema.TypeList,
				Optional: true,
				Computed: true,
				MaxItems: 1,
				DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
					if old == "1" && new == "0" {
						return true
					}
					return false
				},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"flow_logs_enabled": {
							Type:     schema.TypeBool,
							Optional: true,
							Computed: true,
						},
						"flow_logs_s3_bucket": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
						"flow_logs_s3_prefix": {
							Type:     schema.TypeString,
							Optional: true,
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func resourceAwsGlobalAcceleratorAcceleratorCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).globalacceleratorconn

	opts := &globalaccelerator.CreateAcceleratorInput{
		Name:             aws.String(d.Get("name").(string)),
		IdempotencyToken: aws.String(resource.UniqueId()),
		Enabled:          aws.Bool(d.Get("enabled").(bool)),
	}

	if v, ok := d.GetOk("ip_address_type"); ok {
		opts.IpAddressType = aws.String(v.(string))
	}

	log.Printf("[DEBUG] Create Global Accelerator accelerator: %s", opts)

	resp, err := conn.CreateAccelerator(opts)
	if err != nil {
		return fmt.Errorf("Error creating Global Accelerator accelerator: %s", err)
	}

	d.SetId(*resp.Accelerator.AcceleratorArn)

	stateConf := &resource.StateChangeConf{
		Pending: []string{globalaccelerator.AcceleratorStatusInProgress},
		Target:  []string{globalaccelerator.AcceleratorStatusDeployed},
		Refresh: resourceAwsGlobalAcceleratorAcceleratorStateRefreshFunc(conn, d.Id()),
		Timeout: d.Timeout(schema.TimeoutCreate),
	}

	log.Printf("[DEBUG] Waiting for Global Accelerator accelerator (%s) availability", d.Id())
	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error waiting for Global Accelerator accelerator (%s) availability: %s", d.Id(), err)
	}

	if v := d.Get("attributes").([]interface{}); len(v) > 0 {
		err = resourceAwsGlobalAcceleratorAcceleratorUpdateAttributes(conn, d.Id(), v[0].(map[string]interface{}))
		if err != nil {
			return err
		}
	}

	return resourceAwsGlobalAcceleratorAcceleratorRead(d, meta)
}

func resourceAwsGlobalAcceleratorAcceleratorRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).globalacceleratorconn

	accelerator, err := resourceAwsGlobalAcceleratorAcceleratorRetrieve(conn, d.Id())

	if err != nil {
		if isAWSErr(err, globalaccelerator.ErrCodeAcceleratorNotFoundException, "") {
			log.Printf("[WARN] Global Accelerator accelerator (%s) not found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return fmt.Errorf("Error reading Global Accelerator accelerator: %s", err)
	}

	d.Set("name", accelerator.Name)
	d.Set("ip_address_type", accelerator.IpAddressType)
	d.Set("enabled", accelerator.Enabled)
	d.Set("ip_sets", resourceAwsGlobalAcceleratorAcceleratorFlattenIpSets(accelerator.IpSets))

	resp, err := conn.DescribeAcceleratorAttributes(&globalaccelerator.DescribeAcceleratorAttributesInput{
		AcceleratorArn: aws.String(d.Id()),
	})

	if err != nil {
		return fmt.Errorf("Error reading Global Accelerator accelerator attributes: %s", err)
	}

	d.Set("attributes", resourceAwsGlobalAcceleratorAcceleratorFlattenAttributes(resp.AcceleratorAttributes))

	return nil
}

func resourceAwsGlobalAcceleratorAcceleratorFlattenIpSets(ipsets []*globalaccelerator.IpSet) []interface{} {
	out := make([]interface{}, len(ipsets))

	for i, ipset := range ipsets {
		m := make(map[string]interface{})

		m["ip_addresses"] = flattenStringList(ipset.IpAddresses)
		m["ip_family"] = aws.StringValue(ipset.IpFamily)

		out[i] = m
	}

	return out
}

func resourceAwsGlobalAcceleratorAcceleratorFlattenAttributes(attributes *globalaccelerator.AcceleratorAttributes) []interface{} {
	if attributes == nil {
		return nil
	}

	out := make([]interface{}, 1)
	m := make(map[string]interface{})
	m["flow_logs_enabled"] = aws.BoolValue(attributes.FlowLogsEnabled)
	m["flow_logs_s3_bucket"] = aws.StringValue(attributes.FlowLogsS3Bucket)
	m["flow_logs_s3_prefix"] = aws.StringValue(attributes.FlowLogsS3Prefix)
	out[0] = m

	return out
}

func resourceAwsGlobalAcceleratorAcceleratorStateRefreshFunc(conn *globalaccelerator.GlobalAccelerator, acceleratorArn string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		accelerator, err := resourceAwsGlobalAcceleratorAcceleratorRetrieve(conn, acceleratorArn)

		if err != nil {
			log.Printf("Error retrieving Global Accelerator accelerator when waiting: %s", err)
			return nil, "", err
		}

		if accelerator == nil {
			return nil, "", nil
		}

		if accelerator.Status != nil {
			log.Printf("[DEBUG] Global Accelerator accelerator (%s) status : %s", acceleratorArn, aws.StringValue(accelerator.Status))
		}

		return accelerator, aws.StringValue(accelerator.Status), nil
	}
}

func resourceAwsGlobalAcceleratorAcceleratorRetrieve(conn *globalaccelerator.GlobalAccelerator, acceleratorArn string) (*globalaccelerator.Accelerator, error) {
	resp, err := conn.DescribeAccelerator(&globalaccelerator.DescribeAcceleratorInput{
		AcceleratorArn: aws.String(acceleratorArn),
	})

	if err != nil {
		if isAWSErr(err, globalaccelerator.ErrCodeAcceleratorNotFoundException, "") {
			return nil, nil
		}
		return nil, err
	}

	return resp.Accelerator, nil
}

func resourceAwsGlobalAcceleratorAcceleratorUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).globalacceleratorconn

	d.Partial(true)

	if d.HasChange("name") || d.HasChange("ip_address_type") || d.HasChange("enabled") {
		opts := &globalaccelerator.UpdateAcceleratorInput{
			AcceleratorArn: aws.String(d.Id()),
			Name:           aws.String(d.Get("name").(string)),
			Enabled:        aws.Bool(d.Get("enabled").(bool)),
		}

		if v, ok := d.GetOk("ip_address_type"); ok {
			opts.IpAddressType = aws.String(v.(string))
		}

		log.Printf("[DEBUG] Update Global Accelerator accelerator: %s", opts)

		_, err := conn.UpdateAccelerator(opts)
		if err != nil {
			return fmt.Errorf("Error updating Global Accelerator accelerator: %s", err)
		}

		d.SetPartial("name")
		d.SetPartial("ip_address_type")
		d.SetPartial("enabled")

		stateConf := &resource.StateChangeConf{
			Pending: []string{globalaccelerator.AcceleratorStatusInProgress},
			Target:  []string{globalaccelerator.AcceleratorStatusDeployed},
			Refresh: resourceAwsGlobalAcceleratorAcceleratorStateRefreshFunc(conn, d.Id()),
			Timeout: d.Timeout(schema.TimeoutUpdate),
		}

		log.Printf("[DEBUG] Waiting for Global Accelerator accelerator (%s) availability", d.Id())
		_, err = stateConf.WaitForState()
		if err != nil {
			return fmt.Errorf("Error waiting for Global Accelerator accelerator (%s) availability: %s", d.Id(), err)
		}
	}

	if d.HasChange("attributes") {
		if v := d.Get("attributes").([]interface{}); len(v) > 0 {
			err := resourceAwsGlobalAcceleratorAcceleratorUpdateAttributes(conn, d.Id(), v[0].(map[string]interface{}))
			if err != nil {
				return err
			}

		}

		d.SetPartial("attributes")
	}

	d.Partial(false)

	return resourceAwsGlobalAcceleratorAcceleratorRead(d, meta)
}

func resourceAwsGlobalAcceleratorAcceleratorUpdateAttributes(conn *globalaccelerator.GlobalAccelerator, acceleratorArn string, attributes map[string]interface{}) error {
	opts := &globalaccelerator.UpdateAcceleratorAttributesInput{
		AcceleratorArn:  aws.String(acceleratorArn),
		FlowLogsEnabled: aws.Bool(attributes["flow_logs_enabled"].(bool)),
	}

	if v := attributes["flow_logs_s3_bucket"]; v != nil {
		opts.FlowLogsS3Bucket = aws.String(v.(string))
	}

	if v := attributes["flow_logs_s3_prefix"]; v != nil {
		opts.FlowLogsS3Prefix = aws.String(v.(string))
	}

	log.Printf("[DEBUG] Update Global Accelerator accelerator attributes: %s", opts)

	_, err := conn.UpdateAcceleratorAttributes(opts)
	if err != nil {
		return fmt.Errorf("Error updating Global Accelerator accelerator attributes: %s", err)
	}

	return nil
}

func resourceAwsGlobalAcceleratorAcceleratorDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).globalacceleratorconn

	opts := &globalaccelerator.DeleteAcceleratorInput{
		AcceleratorArn: aws.String(d.Id()),
	}

	_, err := conn.DeleteAccelerator(opts)
	if err != nil {
		if isAWSErr(err, globalaccelerator.ErrCodeAcceleratorNotFoundException, "") {
			return nil
		}
		return fmt.Errorf("Error deleting Global Accelerator accelerator: %s", err)
	}

	return nil
}
