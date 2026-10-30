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

import { Dialog } from '@/components/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'

import {
  INVOICE_REGION_LABEL_KEYS,
  INVOICE_REGIONS,
} from '../lib/invoice-title'
import type { InvoiceRegion } from '../types'

// The samples reproduce how the real documents look, so their printed labels
// stay in the document's own language (simplified Chinese for the mainland
// tax e-invoice, traditional Chinese for the Hong Kong INVOICE) whatever the
// interface language is. Only the surrounding UI is translated.

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
              {t(INVOICE_REGION_LABEL_KEYS[region])}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value='mainland'>
          <InvoiceSampleFrame>
            <MainlandInvoiceSample />
          </InvoiceSampleFrame>
        </TabsContent>
        <TabsContent value='hk'>
          <InvoiceSampleFrame>
            <HongKongInvoiceSample />
          </InvoiceSampleFrame>
        </TabsContent>
      </Tabs>
    </Dialog>
  )
}

/** Keeps the paper at its real proportions; narrow screens scroll sideways. */
function InvoiceSampleFrame(props: { children: React.ReactNode }) {
  const { t } = useTranslation()
  return (
    <div
      className='bg-muted/40 overflow-x-auto rounded-lg border p-3'
      data-testid='invoice-sample-frame'
    >
      <div className='mx-auto w-[760px]'>
        {props.children}
        <p className='mt-2 text-center text-xs text-neutral-500'>
          {t('Sample for reference only · not a valid invoice')}
        </p>
      </div>
    </div>
  )
}

const MAINLAND_ITEM_COLUMNS = [
  '项目名称',
  '规格型号',
  '单位',
  '数量',
  '单价',
  '金额',
  '税率/征收率',
  '税额',
]

const mainlandItemGrid =
  'grid grid-cols-[2.2fr_1fr_0.7fr_0.7fr_1fr_1fr_1fr_1fr] px-2'

function VerticalLabel(props: { text: string; className?: string }) {
  return (
    <div
      className={cn(
        'flex w-8 shrink-0 items-center justify-center border-r border-[#a8322d] py-2 text-xs leading-4 [writing-mode:vertical-rl]',
        props.className
      )}
    >
      {props.text}
    </div>
  )
}

export function MainlandInvoiceSample() {
  return (
    <section
      aria-label='电子发票（普通发票）'
      className='border-[#a8322d] bg-white p-6 font-serif text-[#a8322d]'
    >
      <div className='flex items-start justify-between gap-4'>
        <div className='flex gap-3 text-sm'>
          <div className='flex size-20 items-center justify-center border-2 border-current'>
            二维码
          </div>
          <div className='flex size-20 items-center justify-center border-2 border-dashed border-current'>
            标签
          </div>
        </div>
        <div className='flex flex-col items-center'>
          <p className='text-2xl tracking-wider'>电子发票（普通发票）</p>
          <div className='mt-1 h-1.5 w-72 border-y border-current' />
          <div className='-mt-1 flex h-12 w-28 flex-col items-center justify-center rounded-[50%] border-2 border-current bg-white text-[9px] leading-3'>
            <span>全国统一发票监制</span>
            <span>××税务局</span>
          </div>
        </div>
        <div className='space-y-3 pt-2 text-sm'>
          <p>发票号码：</p>
          <p>开票日期：</p>
        </div>
      </div>

      <div className='mt-3 border-2 border-current text-xs'>
        <div className='flex border-b border-current'>
          <VerticalLabel text='购买方信息' />
          <div className='flex-1 space-y-3 p-2'>
            <p>名称：</p>
            <p>统一社会信用代码/纳税人识别号：</p>
          </div>
          <VerticalLabel text='销售方信息' className='border-l' />
          <div className='flex-1 space-y-3 p-2'>
            <p>名称：</p>
            <p>统一社会信用代码/纳税人识别号：</p>
          </div>
        </div>
        <div className={cn(mainlandItemGrid, 'pt-2')}>
          {MAINLAND_ITEM_COLUMNS.map((heading) => (
            <span key={heading} className='text-center'>
              {heading}
            </span>
          ))}
        </div>
        <div className='h-32' />
        <div className={cn(mainlandItemGrid, 'border-b border-current pb-2')}>
          <span className='text-center tracking-[1em]'>合计</span>
        </div>
        <div className='flex border-b border-current'>
          <div className='w-44 border-r border-current p-2 text-center'>
            价税合计（大写）
          </div>
          <div className='flex flex-1 justify-end p-2 pr-24'>（小写）</div>
        </div>
        <div className='flex min-h-20'>
          <VerticalLabel text='备注' />
          <div className='flex-1' />
        </div>
      </div>
      <p className='mt-3 pl-12 text-sm'>开票人：</p>
    </section>
  )
}

function HongKongLine(props: { label: string; strong?: boolean }) {
  return (
    <div className={cn('flex items-end gap-4', props.strong && 'font-bold')}>
      <span className='w-20 shrink-0'>{props.label}</span>
      <span className='h-5 flex-1 border-b border-neutral-300' />
    </div>
  )
}

export function HongKongInvoiceSample() {
  return (
    <section
      aria-label='帳單 INVOICE'
      className='bg-white p-8 text-neutral-900'
    >
      <div className='flex items-start justify-between'>
        <div>
          <p className='text-4xl font-black tracking-wide'>帳單</p>
          <p className='text-sm font-medium'>INVOICE</p>
        </div>
        <p className='text-6xl font-black tracking-tight'>88API</p>
      </div>
      <div className='mt-5 w-80 space-y-2 text-sm'>
        <HongKongLine label='帳單編號' />
        <HongKongLine label='發出日期' />
        <HongKongLine label='到期日' />
      </div>
      <div className='mt-8 grid grid-cols-2 gap-12 text-sm'>
        {['88API', '收件人'].map((heading) => (
          <div key={heading} className='space-y-3'>
            <p className='text-lg font-bold'>{heading}</p>
            <div className='h-4 border-b border-neutral-300' />
            <div className='h-4 border-b border-neutral-300' />
            <div className='h-4 border-b border-neutral-300' />
          </div>
        ))}
      </div>
      <div className='mt-8 flex items-end gap-4'>
        <span className='text-2xl font-black'>應付金額</span>
        <span className='h-6 w-80 border-b border-neutral-300' />
      </div>
      <div className='mt-8 text-sm'>
        <div className='grid grid-cols-[1fr_6rem_6rem_6rem] border-b-2 border-neutral-900 pb-2'>
          <span>描述</span>
          <span className='text-right'>數量</span>
          <span className='text-right'>單價</span>
          <span className='text-right'>金額</span>
        </div>
        <div className='h-9 border-b border-neutral-200' />
        <div className='h-9 border-b border-neutral-200' />
      </div>
      <div className='mt-6 ml-auto w-1/2 space-y-2 text-sm'>
        <HongKongLine label='小計' />
        <HongKongLine label='合計' />
        <HongKongLine label='應付金額' strong />
      </div>
    </section>
  )
}
