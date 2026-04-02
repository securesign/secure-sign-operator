package v1alpha1

// TrillianService implementation of TasService interface
func (i *TrillianService) GetAddress() string {
	return i.Address
}

func (i *TrillianService) GetPort() *int32 {
	return i.Port
}

func (i *TrillianService) SetAddress(address string) {
	i.Address = address
}

func (i *TrillianService) SetPort(port *int32) {
	i.Port = port
}

// TufService implementation of TasService interface
func (i *TufService) GetAddress() string {
	return i.Address
}

func (i *TufService) GetPort() *int32 {
	return i.Port
}

func (i *TufService) SetAddress(address string) {
	i.Address = address
}

func (i *TufService) SetPort(port *int32) {
	i.Port = port
}

// CtlogService implementation of TasService interface
func (i *CtlogService) GetAddress() string {
	return i.Address
}

func (i *CtlogService) GetPort() *int32 {
	return i.Port
}

func (i *CtlogService) SetAddress(address string) {
	i.Address = address
}

func (i *CtlogService) SetPort(port *int32) {
	i.Port = port
}

// FulcioService implementation of TasService interface
func (i *FulcioService) GetAddress() string {
	return i.Address
}

func (i *FulcioService) GetPort() *int32 {
	return i.Port
}

func (i *FulcioService) SetAddress(address string) {
	i.Address = address
}

func (i *FulcioService) SetPort(port *int32) {
	i.Port = port
}

// RekorService implementation of TasService interface
func (i *RekorService) GetAddress() string {
	return i.Address
}

func (i *RekorService) GetPort() *int32 {
	return i.Port
}

func (i *RekorService) SetAddress(address string) {
	i.Address = address
}

func (i *RekorService) SetPort(port *int32) {
	i.Port = port
}

// TsaService implementation of TasService interface
func (i *TsaService) GetAddress() string {
	return i.Address
}

func (i *TsaService) GetPort() *int32 {
	return i.Port
}

func (i *TsaService) SetAddress(address string) {
	i.Address = address
}

func (i *TsaService) SetPort(port *int32) {
	i.Port = port
}
