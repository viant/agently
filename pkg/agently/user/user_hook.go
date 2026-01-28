package user

// SetId sets the id filter.
func (i *UserInput) SetId(v string) {
	i.ensureHas()
	i.Id = v
	i.Has.Id = true
}

// SetUsername sets the username filter.
func (i *UserInput) SetUsername(v string) {
	i.ensureHas()
	i.Username = v
	i.Has.Username = true
}

func (i *UserInput) ensureHas() {
	if i.Has != nil {
		return
	}
	i.Has = &UserInputHas{}
}
