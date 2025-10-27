package kong

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/kong/go-kong/kong"
)

func dataSourceKongConsumer() *schema.Resource {
	return &schema.Resource{
		// This data source only has a Read function
		ReadContext: dataSourceKongConsumerRead,

		// The schema defines the lookup argument and computed attributes
		Schema: map[string]*schema.Schema{
			// --- Lookup Argument ---
			"username": {
				Type:     schema.TypeString,
				Required: true, // This is now the required lookup key
			},

			// --- Computed Attributes ---
			// These fields are populated after the Read operation
			"consumer_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The unique ID of the consumer.",
			},
			"custom_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"tags": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
		},
	}
}

func dataSourceKongConsumerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	client := meta.(*config).adminClient.Consumers

	// 1. Get the username from the configuration
	username := d.Get("username").(string)
	if username == "" {
		// This should not happen if "Required: true" is set
		return diag.FromErr(fmt.Errorf("'username' cannot be empty"))
	}

	// 2. Fetch the consumer from Kong using the username
	// The Kong client's Get() function handily accepts a username OR an ID
	consumer, err := client.Get(ctx, kong.String(username))

	// 3. Handle errors
	if kong.IsNotFoundErr(err) {
		// For a data source, "Not Found" is a fatal error
		return diag.FromErr(fmt.Errorf("could not find Kong Consumer with username: %s", username))
	}
	if err != nil {
		return diag.FromErr(fmt.Errorf("error reading Kong Consumer: %v", err))
	}

	if consumer == nil {
		return diag.FromErr(fmt.Errorf("could not find Kong Consumer with username: %s", username))
	}

	// 4. Set the Terraform state from the API response
	d.SetId(*consumer.ID) // Set the internal TF ID to the Kong ID

	if err := d.Set("consumer_id", consumer.ID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("username", consumer.Username); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("custom_id", consumer.CustomID); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("tags", consumer.Tags); err != nil {
		return diag.FromErr(err)
	}

	return nil
}
