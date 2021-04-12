package blueprint

type Customizations struct {
	Hostname *string                `json:"hostname,omitempty" toml:"hostname,omitempty" hcl:"hostname,optional"`
	Kernel   *KernelCustomization   `json:"kernel,omitempty" toml:"kernel,omitempty" hcl:"kernel,optional"`
	SSHKey   []SSHKeyCustomization  `json:"sshkey,omitempty" toml:"sshkey,omitempty" hcl:"sshkey,optional"`
	User     []UserCustomization    `json:"user,omitempty" toml:"user,omitempty" hcl:"user,optional"`
	Group    []GroupCustomization   `json:"group,omitempty" toml:"group,omitempty" hcl:"group,optional"`
	Timezone *TimezoneCustomization `json:"timezone,omitempty" toml:"timezone,omitempty" hcl:"timezone,optional"`
	Locale   *LocaleCustomization   `json:"locale,omitempty" toml:"locale,omitempty" hcl:"locale,optional"`
	Firewall *FirewallCustomization `json:"firewall,omitempty" toml:"firewall,omitempty" hcl:"firewall,optional"`
	Services *ServicesCustomization `json:"services,omitempty" toml:"services,omitempty" hcl:"services,optional"`
}

type KernelCustomization struct {
	Name   string `json:"name,omitempty" toml:"name,omitempty" hcl:"name,optional"`
	Append string `json:"append" toml:"append" hcl:"append"`
}

type SSHKeyCustomization struct {
	User string `json:"user" toml:"user" hcl:"user"`
	Key  string `json:"key" toml:"key" hcl:"key"`
}

type UserCustomization struct {
	Name        string   `json:"name" toml:"name" hcl:"name"`
	Description *string  `json:"description,omitempty" toml:"description,omitempty" hcl:"description,optional"`
	Password    *string  `json:"password,omitempty" toml:"password,omitempty" hcl:"password,optional"`
	Key         *string  `json:"key,omitempty" toml:"key,omitempty" hcl:"key,optional"`
	Home        *string  `json:"home,omitempty" toml:"home,omitempty" hcl:"home,optional"`
	Shell       *string  `json:"shell,omitempty" toml:"shell,omitempty" hcl:"shell,optional"`
	Groups      []string `json:"groups,omitempty" toml:"groups,omitempty" hcl:"groups,optional"`
	UID         *int     `json:"uid,omitempty" toml:"uid,omitempty" hcl:"uid,optional"`
	GID         *int     `json:"gid,omitempty" toml:"gid,omitempty" hcl:"gid,optional"`
}

type GroupCustomization struct {
	Name string `json:"name" toml:"name" hcl:"name"`
	GID  *int   `json:"gid,omitempty" toml:"gid,omitempty" hcl:"gid,optional"`
}

type TimezoneCustomization struct {
	Timezone   *string  `json:"timezone,omitempty" toml:"timezone,omitempty" hcl:"timezone,optional"`
	NTPServers []string `json:"ntpservers,omitempty" toml:"ntpservers,omitempty" hcl:"ntpservers,optional"`
}

type LocaleCustomization struct {
	Languages []string `json:"languages,omitempty" toml:"languages,omitempty" hcl:"languages,optional"`
	Keyboard  *string  `json:"keyboard,omitempty" toml:"keyboard,omitempty" hcl:"keyboard,optional"`
}

type FirewallCustomization struct {
	Ports    []string                       `json:"ports,omitempty" toml:"ports,omitempty" hcl:"ports,optional"`
	Services *FirewallServicesCustomization `json:"services,omitempty" toml:"services,omitempty" hcl:"services,optional"`
}

type FirewallServicesCustomization struct {
	Enabled  []string `json:"enabled,omitempty" toml:"enabled,omitempty" hcl:"enabled,optional"`
	Disabled []string `json:"disabled,omitempty" toml:"disabled,omitempty" hcl:"disabled,optional"`
}

type ServicesCustomization struct {
	Enabled  []string `json:"enabled,omitempty" toml:"enabled,omitempty" hcl:"enabled,optional"`
	Disabled []string `json:"disabled,omitempty" toml:"disabled,omitempty" hcl:"disabled,optional"`
}

type CustomizationError struct {
	Message string
}

func (e *CustomizationError) Error() string {
	return e.Message
}

func (c *Customizations) GetHostname() *string {
	if c == nil {
		return nil
	}
	return c.Hostname
}

func (c *Customizations) GetPrimaryLocale() (*string, *string) {
	if c == nil {
		return nil, nil
	}
	if c.Locale == nil {
		return nil, nil
	}
	if len(c.Locale.Languages) == 0 {
		return nil, c.Locale.Keyboard
	}
	return &c.Locale.Languages[0], c.Locale.Keyboard
}

func (c *Customizations) GetTimezoneSettings() (*string, []string) {
	if c == nil {
		return nil, nil
	}
	if c.Timezone == nil {
		return nil, nil
	}
	return c.Timezone.Timezone, c.Timezone.NTPServers
}

func (c *Customizations) GetUsers() []UserCustomization {
	if c == nil {
		return nil
	}

	users := []UserCustomization{}

	// prepend sshkey for backwards compat (overridden by users)
	if len(c.SSHKey) > 0 {
		for _, c := range c.SSHKey {
			users = append(users, UserCustomization{
				Name: c.User,
				Key:  &c.Key,
			})
		}
	}

	return append(users, c.User...)
}

func (c *Customizations) GetGroups() []GroupCustomization {
	if c == nil {
		return nil
	}

	// This is for parity with lorax, which assumes that for each
	// user, a group with that name already exists. Thus, filter groups
	// named like an existing user.

	groups := []GroupCustomization{}
	for _, group := range c.Group {
		exists := false
		for _, user := range c.User {
			if user.Name == group.Name {
				exists = true
				break
			}
		}
		for _, key := range c.SSHKey {
			if key.User == group.Name {
				exists = true
				break
			}
		}
		if !exists {
			groups = append(groups, group)
		}
	}

	return groups
}

func (c *Customizations) GetKernel() *KernelCustomization {
	var name string
	var append string
	if c != nil && c.Kernel != nil {
		name = c.Kernel.Name
		append = c.Kernel.Append
	}

	if name == "" {
		name = "kernel"
	}

	return &KernelCustomization{
		Name:   name,
		Append: append,
	}
}

func (c *Customizations) GetFirewall() *FirewallCustomization {
	if c == nil {
		return nil
	}

	return c.Firewall
}

func (c *Customizations) GetServices() *ServicesCustomization {
	if c == nil {
		return nil
	}

	return c.Services
}
