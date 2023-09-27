// Copyright © 2023 sealos.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resources

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/labring/sealos/controllers/pkg/crypto"
	"github.com/labring/sealos/pkg/utils/logger"

	accountv1 "github.com/labring/sealos/controllers/account/api/v1"

	"github.com/labring/sealos/controllers/pkg/gpu"
	"github.com/labring/sealos/controllers/pkg/utils/env"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CategoryField = "category"
	PropertyField = "property"
	TimeField     = "time"
	ValueField    = "value"
	PriceField    = "price"
	AmountField   = "amount"
)

/*
Unified base unit:

CPU: millicore (mCore) (1 core = 1000 millicores) 0.067 / 1000 * 1000000 = 67 unit price
Memory: Mebibytes (MiB) 0.033 / 1024 (2^30) * 1000000 = 33 unit price
Storage: Mebibytes (MiB) 0.0021 / 1024 * 1000000 = 2 unit price
Network bandwidth: Kbps (kilobits per second) not yet available
*/

//| property     | Price | Detail         |
//| ------------ | ----- | -------------- |
//| Cpu          | 67    | mCore unit     |
//| Memory       | 33    | Mebibytes unit |
//| Disk         | 2     | Mebibytes unit |
//| Mongodb      | 1     | feature...     |
//| Minio        | 1     | Mebibytes unit |
//| Infra-Cpu    | 67    | mCore unit     |
//| Infra-Memory | 33    | Mebibytes unit |
//| Infra-Disk   | 2     | Mebibytes unit |
//
// price: 1000000 = 1¥

type Price struct {
	Property string `json:"property" bson:"property"`
	Price    int64  `json:"price" bson:"price"`
	Detail   string `json:"detail" bson:"detail"`
	//Unit     string  `json:"unit" bson:"unit"`
}

//| Category   | property     | Time       | value |      |
//| ---------- | ------------ | -------    | ----- | ---- |
//| Namespace1 | cpu          | timestamp1 | 5     |      |
//| Namespace1 | memory       | timestamp1 | 100   |      |
//| Namespace1 | disk         | timestamp1 | 30    |      |
//| appid1     | mongodb      | timestamp1 | 100   |      |
//| appid1     | Minio        | timestamp1 | 100   |      |
//| Namespace2 | Infra-Cpu    | timestamp1 | 5     |      |
//| Namespace2 | cpu          | timestamp1 | 100   |      |
//| Namespace2 | Infra-Memory | timestamp1 | 30    |      |
//| Namespace2 | Infra-Disk   | timestamp1 | 100   |      |
//| Namespace2 | memory       | timestamp1 | 100   |      |
//| Namespace2 |  disk        | timestamp1 | 100   |      |

// Composite index: category, property, time, speed up query
type Monitor struct {
	Time time.Time `json:"time" bson:"time"`
	// equal namespace
	Category string      `json:"category" bson:"category"`
	Type     uint8       `json:"type" bson:"type"`
	Name     string      `json:"name" bson:"name"`
	Used     EnumUsedMap `json:"used" bson:"used"`
	Property string      `json:"property" bson:"property"`
}

type BillingType int

type Billing struct {
	Time    time.Time      `json:"time" bson:"time"`
	OrderID string         `json:"order_id" bson:"order_id"`
	Type    accountv1.Type `json:"type" bson:"type"`
	//Name      string      `json:"name" bson:"name"`
	Namespace string `json:"namespace" bson:"namespace"`
	//Used       Used        `json:"used" bson:"used"`
	//UsedAmount Used        `json:"used_amount" bson:"used_amount"`

	AppCosts []AppCost `json:"app_costs,omitempty" bson:"app_costs,omitempty"`
	AppType  uint8     `json:"app_type,omitempty" bson:"app_type,omitempty"`

	Amount int64  `json:"amount" bson:"amount,omitempty"`
	Owner  string `json:"owner" bson:"owner,omitempty"`
	// 0: 未结算 1: 已结算
	Status BillingStatus `json:"status" bson:"status,omitempty"`
	// if type = Consumption, then payment is not nil
	Payment *Payment `json:"payment" bson:"payment,omitempty"`
	// if type = Transfer, then transfer is not nil
	Transfer *Transfer `json:"transfer" bson:"transfer,omitempty"`
}

type Payment struct {
	Method  string `json:"method" bson:"method"`
	UserID  string `json:"user_id" bson:"user_id"`
	Amount  int64  `json:"amount,omitempty"`
	TradeNO string `json:"tradeNO,omitempty"`
	// CodeURL is the codeURL of wechatpay
	CodeURL string `json:"codeURL,omitempty"`
}

type Transfer struct {
	From   string `json:"from" bson:"from,omitempty"`
	To     string `json:"to" bson:"to,omitempty"`
	Amount int64  `json:"amount" bson:"amount"`
}

type AppCost struct {
	Used       EnumUsedMap `json:"used" bson:"used"`
	UsedAmount EnumUsedMap `json:"used_amount" bson:"used_amount"`
	Amount     int64       `json:"amount" bson:"amount,omitempty"`
	Name       string      `json:"name" bson:"name"`
}

type BillingHandler struct {
	OrderID string        `json:"order_id" bson:"order_id"`
	Time    time.Time     `json:"time" bson:"time"`
	Amount  int64         `json:"amount" bson:"amount,omitempty"`
	Status  BillingStatus `json:"status" bson:"status,omitempty"`
}

type BillingStatus int

const (
	// 0: 未结算 1: 已结算
	Unsettled BillingStatus = iota
	Settled
)

const (
	// 	DB       = 1
	//	APP      = 2
	//	TERMINAL = 3
	//	JOB      = 4
	//	OTHER    = 5

	db = iota + 1
	app
	terminal
	job
	other
)

const (
	DB       = "DB"
	APP      = "APP"
	TERMINAL = "TERMINAL"
	JOB      = "JOB"
	OTHER    = "OTHER"
)

var AppType = map[string]uint8{
	DB: db, APP: app, TERMINAL: terminal, JOB: job, OTHER: other,
}

var AppTypeReverse = map[uint8]string{
	db: DB, app: APP, terminal: TERMINAL, job: JOB, other: OTHER,
}

// 资源消耗
type EnumUsedMap map[uint8]int64

type PropertyType struct {
	// 对应监控存储枚举类型，使用uint8，可以节省内存
	// 0 cpu, 1 memory, 2 storage, 3 network ... 可扩展
	Name  string `json:"name" bson:"name"`
	Alias string `json:"alias" bson:"alias"`
	Enum  uint8  `json:"enum" bson:"enum"`
	//平均值，累加值 默认为平均值
	//AVG , SUM
	PriceType string `json:"price_type,omitempty" bson:"price_type,omitempty"`
	// Price = UsedAmount (平均值||累加值) / Unit * UnitPrice
	UnitPrice        int64             `json:"unit_price" bson:"unit_price"`
	EncryptUnitPrice string            `json:"encrypt_unit_price" bson:"encrypt_unit_price"`
	Unit             resource.Quantity `json:"-" bson:"-"`
	// <digit>           ::= 0 | 1 | ... | 9
	// <digits>          ::= <digit> | <digit><digits>
	// <number>          ::= <digits> | <digits>.<digits> | <digits>. | .<digits>
	// <sign>            ::= "+" | "-"
	// <signedNumber>    ::= <number> | <sign><number>
	// <suffix>          ::= <binarySI> | <decimalExponent> | <decimalSI>
	// <binarySI>        ::= Ki | Mi | Gi | Ti | Pi | Ei
	//
	//	(International System of units; See: http://physics.nist.gov/cuu/Units/binary.html)
	//
	// <decimalSI>       ::= m | "" | k | M | G | T | P | E
	//
	//	(Note that 1024 = 1Ki but 1000 = 1k; I didn't choose the capitalization.)
	//
	// <decimalExponent> ::= "e" <signedNumber> | "E" <signedNumber>
	UnitString string `json:"unit" bson:"unit"`
	//计费周期 second
	UnitPeriod string `json:"unit_period,omitempty" bson:"unit_period,omitempty"`
}

type PropertyTypeLS struct {
	Types     []PropertyType
	StringMap map[string]PropertyType
	EnumMap   map[uint8]PropertyType
}

var DefaultPropertyTypeList = []PropertyType{
	{
		Name:       "cpu",
		Enum:       0,
		PriceType:  "AVG",
		UnitPrice:  67,
		UnitString: "1m",
	},
	{
		Name:       "memory",
		Enum:       1,
		PriceType:  "AVG",
		UnitPrice:  33,
		UnitString: "1Mi",
	},
	{
		Name:       "storage",
		Enum:       2,
		PriceType:  "AVG",
		UnitPrice:  2,
		UnitString: "1Mi",
	},
	{
		Name:       "network",
		Enum:       3,
		PriceType:  "AVG",
		UnitPrice:  781,
		UnitString: "1Mi",
	},
}

var DefaultPropertyTypeLS = newPropertyTypeLS(DefaultPropertyTypeList)

func ConvertEnumUsedToString(costs map[uint8]int64) (costsMap map[string]int64) {
	costsMap = make(map[string]int64, len(costs))
	for k, v := range costs {
		costsMap[DefaultPropertyTypeLS.EnumMap[k].Name] = v
	}
	return
}

func NewPropertyTypeLS(types []PropertyType) (ls *PropertyTypeLS) {
	types, err := decryptPrice(types)
	if err != nil {
		logger.Warn("failed to decrypt price : %v", err)
		types = DefaultPropertyTypeList
	}
	return newPropertyTypeLS(types)
}

func newPropertyTypeLS(types []PropertyType) (ls *PropertyTypeLS) {
	ls = &PropertyTypeLS{
		Types:     types,
		StringMap: make(PropertyTypeStringMap, len(types)),
		EnumMap:   make(PropertyTypeEnumMap, len(types)),
	}
	for i := range types {
		if types[i].Unit == (resource.Quantity{}) && types[i].UnitString != "" {
			types[i].Unit = resource.MustParse(types[i].UnitString)
		}
		ls.EnumMap[types[i].Enum] = types[i]
		ls.StringMap[types[i].Name] = types[i]
	}
	return
}

func decryptPrice(types []PropertyType) ([]PropertyType, error) {
	for i := range types {
		if types[i].EncryptUnitPrice == "" {
			return types, fmt.Errorf("encrypt %s unit price is empty", types[i].Name)
		}
		price, err := crypto.DecryptInt64(types[i].EncryptUnitPrice)
		if err != nil {
			return types, fmt.Errorf("failed to decrypt %s unit price : %v", types[i].Name, err)
		}
		types[i].UnitPrice = price
		//if types[i].UnitPrice != 0 {
		//	price, err := crypto.EncryptInt64(types[i].UnitPrice)
		//	if err != nil {
		//		logger.Error("failed to encrypt unit price : %v", err)
		//	} else {
		//		types[i].EncryptUnitPrice = *price
		//	}
		//}
	}
	return types, nil
}

type PropertyTypeEnumMap map[uint8]PropertyType

type PropertyTypeStringMap map[string]PropertyType

type PropertyList []PropertyType

// | Category   | Property | Time       | Value (average value) | Amount (value * price) | Detail | Status |
// | ---------- | -------- | ---------- | --------------------- | ---------------------- | ------ | ------ |
// | Namespace1 | Cpu      | 2023010112 | 1000                  | 67000                  |        | 0      |
type Metering struct {
	Category string `json:"category" bson:"category"`
	//time 2023010112 -> 2023-01-01 11:00:00 - 2023-01-01 12:00:00
	Amount int64 `json:"amount" bson:"amount"`
	// 2023010112 -> 2023-01-01 12:00:00
	Property string    `json:"property" bson:"property"`
	Value    int64     `json:"value" bson:"value"`
	Time     time.Time `json:"time" bson:"time"`
	Detail   string    `json:"detail" bson:"detail"`
	// 0 -> not settled, 1 -> settled, -1 -> deleted, -2 -> refunded
	//Status int `json:"status" bson:"status"`
}
type QuantityDetail struct {
	*resource.Quantity
	Detail string
}

const (
	SealosResourcesDBName        = "sealos-resources"
	SealosMonitorCollectionName  = "monitor"
	SealosPricesCollectionName   = "prices"
	SealosMeteringCollectionName = "metering"
	SealosBillingCollectionName  = "billing"
)
const (
	PropertyInfraCPU    = "infra-cpu"
	PropertyInfraMemory = "infra-memory"
	PropertyInfraDisk   = "infra-disk"
)

// GpuResourcePrefix GPUResource = gpu- + gpu.Product ; ex. gpu-tesla-v100
const GpuResourcePrefix = "gpu-"

const ResourceGPU corev1.ResourceName = gpu.NvidiaGpuKey
const ResourceNetwork = "network"

const (
	ResourceRequestGpu corev1.ResourceName = "requests." + gpu.NvidiaGpuKey
	ResourceLimitGpu   corev1.ResourceName = "limits." + gpu.NvidiaGpuKey
)

func NewGpuResource(product string) corev1.ResourceName {
	return corev1.ResourceName(GpuResourcePrefix + product)
}
func IsGpuResource(resource string) bool {
	return strings.HasPrefix(resource, GpuResourcePrefix)
}
func GetGpuResourceProduct(resource string) string {
	return strings.TrimPrefix(resource, GpuResourcePrefix)
}

var (
	bin1Mi  = resource.NewQuantity(1<<20, resource.BinarySI)
	cpuUnit = resource.MustParse("1m")
)
var PricesUnit = map[corev1.ResourceName]*resource.Quantity{
	corev1.ResourceCPU:     &cpuUnit, // 1 m CPU (1000 μ)
	ResourceGPU:            &cpuUnit, // 1 m CPU (1000 μ)
	corev1.ResourceMemory:  bin1Mi,   // 1 MiB
	corev1.ResourceStorage: bin1Mi,   // 1 MiB
	ResourceNetwork:        bin1Mi,   // 1 MiB
}

var DefaultPrices = map[string]Price{
	"cpu": {
		Property: "cpu",
		Price:    67,
	},
	"memory": {
		Property: "memory",
		Price:    33,
	},
	"storage": {
		Property: "storage",
		Price:    2,
	},
}

// Core
var infraCPUMap = map[string]int{
	"t2.medium":     2,
	"t2.large":      2,
	"t2.xlarge":     4,
	"ecs.c7.large":  2,
	"ecs.g7.large":  2,
	"ecs.g7.xlarge": 4,
}

// GiB
var infraMemoryMap = map[string]int{
	"t2.medium":     4,
	"t2.large":      8,
	"t2.xlarge":     16,
	"ecs.c7.large":  4,
	"ecs.g7.large":  8,
	"ecs.g7.xlarge": 16,
}

func GetDefaultResourceQuota(ns, name string) *corev1.ResourceQuota {
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: DefaultResourceQuotaHard(),
		},
	}
}

func GetDefaultLimitRange(ns, name string) *corev1.LimitRange {
	return &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.LimitRangeSpec{
			Limits: DefaultLimitRangeLimits(),
		},
	}
}

const (
	QuotaLimitsCPU       = "QUOTA_LIMITS_CPU"
	QuotaLimitsMemory    = "QUOTA_LIMITS_MEMORY"
	QuotaLimitsStorage   = "QUOTA_LIMITS_STORAGE"
	QuotaLimitsGPU       = "QUOTA_LIMITS_GPU"
	QuotaLimitsNodePorts = "QUOTA_LIMITS_NODE_PORTS"
)

const (
	DefaultQuotaLimitsCPU       = "16"
	DefaultQuotaLimitsMemory    = "64Gi"
	DefaultQuotaLimitsStorage   = "100Gi"
	DefaultQuotaLimitsGPU       = "8"
	DefaultQuotaLimitsNodePorts = "3"
)

func DefaultResourceQuotaHard() corev1.ResourceList {
	return corev1.ResourceList{
		ResourceRequestGpu:                    resource.MustParse(env.GetEnvWithDefault(QuotaLimitsGPU, DefaultQuotaLimitsGPU)),
		ResourceLimitGpu:                      resource.MustParse(env.GetEnvWithDefault(QuotaLimitsGPU, DefaultQuotaLimitsGPU)),
		corev1.ResourceLimitsCPU:              resource.MustParse(env.GetEnvWithDefault(QuotaLimitsCPU, DefaultQuotaLimitsCPU)),
		corev1.ResourceLimitsMemory:           resource.MustParse(env.GetEnvWithDefault(QuotaLimitsMemory, DefaultQuotaLimitsMemory)),
		corev1.ResourceRequestsStorage:        resource.MustParse(env.GetEnvWithDefault(QuotaLimitsStorage, DefaultQuotaLimitsStorage)),
		corev1.ResourceLimitsEphemeralStorage: resource.MustParse(env.GetEnvWithDefault(QuotaLimitsStorage, DefaultQuotaLimitsStorage)),
		corev1.ResourceServicesNodePorts:      resource.MustParse(DefaultQuotaLimitsNodePorts),
		//TODO storage.diskio.read, storage.diskio.write
	}
}

func DefaultLimitRangeLimits() []corev1.LimitRangeItem {
	return []corev1.LimitRangeItem{
		{
			Type: corev1.LimitTypeContainer,
			Default: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
}

// MiB
func GetInfraCPUQuantity(flavor string, count int) *resource.Quantity {
	if v, ok := infraCPUMap[flavor]; ok {
		return resource.NewQuantity(int64(v*count), resource.DecimalSI)
	}
	return nil
}

// Gib
func GetInfraMemoryQuantity(flavor string, count int) *resource.Quantity {
	if v, ok := infraMemoryMap[flavor]; ok {
		return resource.NewQuantity(int64((v*count)<<30), resource.BinarySI)
	}
	return nil
}

// Gib
func GetInfraDiskQuantity(capacity int) *resource.Quantity {
	return resource.NewQuantity(int64(capacity<<30), resource.BinarySI)
}

func GetResourceValue(resourceName corev1.ResourceName, res map[corev1.ResourceName]*QuantityDetail) int64 {
	quantity := res[resourceName]
	if quantity != nil && quantity.MilliValue() != 0 {
		return int64(math.Ceil(float64(quantity.MilliValue()) / float64(PricesUnit[resourceName].MilliValue())))
	}
	return 0
}

func GetPrices(mongoClient *mongo.Client) ([]Price, error) {
	collection := mongoClient.Database(SealosResourcesDBName).Collection(SealosPricesCollectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("get all prices error: %v", err)
	}
	var prices []Price
	if err = cursor.All(ctx, &prices); err != nil {
		return nil, fmt.Errorf("get all prices error: %v", err)
	}
	return prices, nil
}