package domain

import "time"

type OrderStatus string

const (
	OrderAwaitingShipment OrderStatus = "awaiting_shipment" //等待发货
	OrderCancelled        OrderStatus = "cancelled"         //已取消
)

type OrderItem struct {
	SKU      string //商品规格标识,比如iphone16 pro max -256G-蓝色
	Quantity int
	Price    float64
}

type Order struct {
	ID               string
	Market           string  //市场,如美国
	Currency         string  //币种
	Amount           float64 //金额
	Status           OrderStatus
	FulfillmentWH    string //仓库标识
	CancellationOpen bool   //是否开放取消操作
	Items            []OrderItem
}

// InventoryBalance 表示某个仓库中某个 SKU 的可用库存。
type InventoryBalance struct {
	WarehouseID string
	SKU         string
	Available   int
}

// InventoryTransfer 表示一次跨仓库存调拨记录。
type InventoryTransfer struct {
	ID            string
	SKU           string
	FromWarehouse string
	ToWarehouse   string
	Quantity      int
	Status        string
	CreatedAt     time.Time
}


