package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

const OC_URL = `https://fbusinesscenter.com/purchase-orders`
const purchaseCardSelector = `a[href="/purchase-orders"]`

type OCNro string

type Store interface {
	IsProcessed(oc OCNro) bool
}

type fakeStore map[OCNro]bool

// IsProcessed implements [Store].
func (f fakeStore) IsProcessed(oc OCNro) bool {
	if _, ok := f[oc]; ok {
		return true
	}
	f[oc] = true
	return false
}

var _ Store = (*fakeStore)(nil)

func CollectOC(ctx context.Context, page *rod.Page, s Store) error {
	var err error

	loginEl, err := page.Timeout(20 * time.Second).Element(`a[href="/login"]`)
	if err == nil {
		waitNav := page.MustWaitNavigation()
		loginEl.MustClick()
		waitNav()
	}

	// Buscar y hacer clic en la tarjeta de Purchase Orders
	waitNav := page.MustWaitNavigation()
	card, err := page.Timeout(15 * time.Second).Element(purchaseCardSelector)
	if err != nil {
		return fmt.Errorf("no se encontró la tarjeta de purchase orders: %w", err)
	}

	if err = card.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("no se pudo hacer clic en la tarjeta: %w", err)
	}
	waitNav() // Esperamos a que cargue la vista de órdenes

	for {
		// Como es una SPA, hacemos clic en la pestaña de historial directamente
		page.MustElement(historycOCTabSelector).MustClick()

		err = page.Timeout(15 * time.Second).Wait(rod.Eval(`() => {
			const rows = document.querySelectorAll('table[aria-labelledby="tableTitle"] tbody tr');
			return rows.length > 0;
		}`))

		if err != nil {
			return fmt.Errorf("timeout: la tabla no cargó filas en 10 segundos: %w", err)
		}

		fmt.Println("¡Secuencia completada y tabla cargada con elementos!")

		orders, err := ExtractPurchaseOrders(page)
		if err != nil {
			return err
		}

		// Bandera para saber si procesamos alguna orden en esta pasada
		processedOne := false
		for currentRow, o := range orders {
			if s.IsProcessed(OCNro(o.OrderNumber)) {
				continue
			}

			if strings.ToLower(o.Status) == "canceled" {
				continue
			}

			if o.AgreementPercentage > 0 {
				continue
			}
			fmt.Println(o)

			openRowSelector := fmt.Sprintf(
				`div[Id="single-spa-application:purchaseOrders"] tbody tr:nth-of-type(%d) button`,
				currentRow+1,
			)

			wait := page.Timeout(20 * time.Second).MustWaitNavigation()
			page.MustElement(openRowSelector).MustClick()
			wait()

			downloadBtns := page.MustElements(`button[Id="donwload-button"]`)
			downloadBtns[0].MustClick()
			time.Sleep(5 * time.Second)
			page.MustSearch("Excel").MustClick()

			downloadBtns[1].MustClick()
			time.Sleep(5 * time.Second)
			page.MustSearch("Excel").MustClick()

			// refactorizar en el futuro
			time.Sleep(10 * time.Second)

			wait = page.MustWaitNavigation()
			page.MustNavigate(OC_URL)
			wait()

			processedOne = true
		}
		if !processedOne {
			break
		}
	}
	return nil
}

const historycOCTabSelector = `div[id="single-spa-application:purchaseOrders"] div[aria-label="po tabs"][role="tablist"] button:nth-of-type(2)`

const OCHistoryTableSelector = `table[aria-labelledby="tableTitle"]`

type PurchaseOrder struct {
	BU                  string
	OrderNumber         string
	CreationDate        string
	DeliveryStartDate   string
	DeliveryEndDate     string
	DeliveryType        string
	Delivery            string
	TotalAmount         string
	TotalQuantity       string
	AgreementPercentage float64
	Status              string
}

func ExtractPurchaseOrders(page *rod.Page) ([]PurchaseOrder, error) {
	// 1. Obtenemos todas las filas (tr) del cuerpo de la tabla usando el auto-wait de Rod
	rows, err := page.Elements(`table[aria-labelledby="tableTitle"] tbody tr`)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron encontrar las filas de la tabla: %w", err)
	}

	var orders []PurchaseOrder

	// 2. Iteramos sobre cada fila de forma totalmente nativa en Go
	for _, row := range rows {
		// Obtenemos todas las celdas (td) de la fila actual
		cells, err := row.Elements(`td`)
		if err != nil || len(cells) < 11 {
			continue // Si la fila no tiene suficientes celdas, la ignoramos de forma segura
		}

		// 3. Extraemos el texto celda por celda usando el método .Text() de go-rod
		bu, _ := cells[0].Text()
		orderNumber, _ := cells[1].Text()
		creationDate, _ := cells[2].Text()
		deliveryStartDate, _ := cells[3].Text()
		deliveryEndDate, _ := cells[4].Text()
		deliveryType, _ := cells[5].Text()
		delivery, _ := cells[6].Text()
		totalAmount, _ := cells[7].Text()
		totalQuantity, _ := cells[8].Text()
		agreementPercentage, _ := cells[9].Text()
		status, _ := cells[10].Text()

		agreementPercentage = strings.ReplaceAll(agreementPercentage, "%", "")
		agreementPercentage = strings.ReplaceAll(agreementPercentage, ",", ".")

		parsedPercentage, err := strconv.ParseFloat(agreementPercentage, 64)
		if err != nil {
			log.Println(err)
			continue
		}

		orders = append(orders, PurchaseOrder{
			BU:                  bu,
			OrderNumber:         orderNumber,
			CreationDate:        creationDate,
			DeliveryStartDate:   deliveryStartDate,
			DeliveryEndDate:     deliveryEndDate,
			DeliveryType:        deliveryType,
			Delivery:            delivery,
			TotalAmount:         totalAmount,
			TotalQuantity:       totalQuantity,
			AgreementPercentage: parsedPercentage,
			Status:              status,
		})
	}

	return orders, nil
}
