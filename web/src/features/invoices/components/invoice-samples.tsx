/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'

import hongKongSample from '@/assets/invoice-samples/hong-kong.webp'
import mainlandSample from '@/assets/invoice-samples/mainland.jpg'
import { Dialog } from '@/components/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  INVOICE_REGION_LABEL_KEYS,
  INVOICE_REGION_SHORT_LABEL_KEYS,
  INVOICE_REGIONS,
} from '../lib/invoice-title'
import type { InvoiceRegion } from '../types'

// Blank sample documents supplied by the business, shown as-is so customers
// see exactly what they will receive. The mainland one is the general
// (普通发票) e-invoice, the only mainland type we issue.
const INVOICE_SAMPLE_IMAGES: Record<InvoiceRegion, string> = {
  mainland: mainlandSample,
  hk: hongKongSample,
}

type InvoiceSampleDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  region: InvoiceRegion
  onRegionChange: (region: InvoiceRegion) => void
}

export function InvoiceSampleDialog(props: InvoiceSampleDialogProps) {
  const { t } = useTranslation()
  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Invoice samples')}
      description={t(
        'See what each invoice type looks like before you apply. Issued invoices carry your billing details and amounts.'
      )}
      contentClassName='sm:max-w-4xl'
    >
      <Tabs
        value={props.region}
        onValueChange={(value) => props.onRegionChange(value as InvoiceRegion)}
      >
        <TabsList className='h-auto max-w-full flex-wrap'>
          {INVOICE_REGIONS.map((region) => (
            <TabsTrigger
              key={region}
              value={region}
              className='whitespace-normal'
            >
              {t(INVOICE_REGION_SHORT_LABEL_KEYS[region])}
            </TabsTrigger>
          ))}
        </TabsList>
        {INVOICE_REGIONS.map((region) => (
          <TabsContent key={region} value={region}>
            <div
              className='bg-muted/40 rounded-lg border p-3'
              data-testid='invoice-sample-frame'
            >
              <img
                src={INVOICE_SAMPLE_IMAGES[region]}
                alt={t(INVOICE_REGION_LABEL_KEYS[region])}
                className='mx-auto block h-auto w-full max-w-[760px] bg-white'
              />
              <p className='mt-2 text-center text-xs text-neutral-500'>
                {t('Sample for reference only · not a valid invoice')}
              </p>
            </div>
          </TabsContent>
        ))}
      </Tabs>
    </Dialog>
  )
}
