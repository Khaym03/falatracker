// Package falabella is the adapter that turns the two reports downloaded from
// the Falabella portal into the standard Excel file (package standard).
//
// # Input files
//
//   - oc-<N>-products-details-<date>.xlsx: one row per product of the
//     purchase order. This is the file that becomes the standard file.
//   - oc-<N>-distributions-details-<date>.xlsx: the same products split by
//     store. It is NOT loaded into Dynamics (one invoice covers every store);
//     it is kept to attach to the invoice e-mail and used here only to
//     cross-check quantities.
//
// Columns of both files are found by header name, so Falabella can reorder
// them without breaking the adapter.
//
// # Mapping products-details -> standard
//
//	Standard column       Source                              Rule
//	--------------------  ----------------------------------  -----------------------------------------
//	RUC                   Customer.TaxID                      placeholder until the NIT is known (1)
//	CLIENTE               Customer.Name                       placeholder (1)
//	N° ORDEN              Número OC
//	FECHA ORDEN           Fecha de emisión
//	FECHA ENTREGA         Fecha fin recepción
//	CONDICIÓN PAGO        Forma de pago, else Customer        usually empty in the file (1)
//	MONEDA                Moneda                              new column (COP for Colombia)
//	MARCA                 Marca                               one invoice per brand (done downstream)
//	CÓD. PRODUCTO         VIN                                 last 6 digits, see ProductCode
//	CÓD. BARRAS           UPC
//	DESCRIPCIÓN           Descripción
//	CANTIDAD              Unidades compradas                  cancelled units only noted (2)
//	PRECIO UNIT.          Costo                               NOT "Precio de venta"
//	DESCUENTO 1 % / 2 %   -                                   0; line charges/discounts are flagged (3)
//	SUBTOTAL              Costo × Unidades compradas
//	IVA %                 Customer.VATPct                     unknown yet -> empty (1)
//	TOTAL                 SUBTOTAL × (1 + IVA)                = SUBTOTAL while IVA is unknown
//	CATEGORÍA             Descripción clasificación
//	DOCUMENTO ORIGEN      products-details file name
//	No, CODIGO_PADRE,     Catalog (NAOS product master)       pending for Colombia: NoCatalog (1)
//	Precio/Descuento/
//	Subtotal Naos,
//	Diferencia
//	TAMAÑO, UXC,          -                                   not provided by Falabella
//	REG. SANITARIO
//
//	(1) Data still missing; the extension points are Customer and Catalog.
//	(2) Not confirmed whether CANTIDAD should subtract "Unidades canceladas".
//	(3) Always 0 so far; the format of these amounts is unknown.
//
// # Business rules (meeting 2026-10-30)
//
//   - An order already in the store is never processed again (tracker).
//   - Only orders with 0% agreement progress are new (order.NeedsReports).
//   - Billing date: see BillingDate. It is not written to the standard file
//     (which only holds data loaded into Dynamics); it goes to
//     order.Record.BillingDate, which PAD reads from GET /orders.
//   - One invoice per brand: the MARCA column lets the loader group lines.
//
// # Checks
//
// Problems never stop the conversion; they are reported so PAD and the
// operators can decide:
//
//   - line level, in the Error/OBSERVACION columns (same wording as Core.py);
//   - order level, in Result.Issues: totals vs "Total OC bruto", units per
//     SKU vs the distribution file, placeholder customer data.
package falabella
