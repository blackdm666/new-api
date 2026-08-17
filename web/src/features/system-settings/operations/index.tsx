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
import { useStatus } from '@/hooks/use-status'

import { SettingsPage } from '../components/settings-page'
import type { OperationsSettings } from '../types'
import {
  OPERATIONS_DEFAULT_SECTION,
  getOperationsSectionContent,
  getOperationsSectionMeta,
} from './section-registry.tsx'

const defaultOperationsSettings: OperationsSettings = {
  DefaultCollapseSidebar: false,
  DemoSiteEnabled: false,
  SelfUseModeEnabled: false,
  QuotaRemindThreshold: '',
  SMTPServer: '',
  SMTPPort: '',
  SMTPAccount: '',
  SMTPFrom: '',
  SMTPToken: '',
  SMTPSSLEnabled: false,
  SMTPStartTLSEnabled: false,
  SMTPInsecureSkipVerify: false,
  SMTPForceAuthLogin: false,
  SMTPBackupEnabled: false,
  SMTPBackupServer: '',
  SMTPBackupPort: '587',
  SMTPBackupAccount: '',
  SMTPBackupFrom: '',
  SMTPBackupToken: '',
  SMTPBackupSSLEnabled: false,
  SMTPBackupStartTLSEnabled: false,
  SMTPBackupInsecureSkipVerify: false,
  SMTPBackupForceAuthLogin: false,
  InvoiceApplicationNotifyAdminEnabled: false,
  InvoiceIssuedNotifyUserEnabled: false,
  InvoiceAdminEmail: '',
  InvoiceMinimumAmountCents: '50000',
  InvoiceDataRetentionDays: '0',
  InvoicePendingExpiryDays: '30',
  InvoiceFileEnabled: true,
  InvoiceFileStorage: 'local',
  InvoiceFileMaxSize: '5242880',
  InvoiceFileMaxCount: '5',
  InvoiceFileAllowedExts: 'jpg,jpeg,png,webp,pdf',
  InvoiceFileLocalPath: '/data/invoice_files',
  InvoiceFileSignedURLTTL: '900',
  InvoiceFileOSSEndpoint: '',
  InvoiceFileOSSBucket: '',
  InvoiceFileOSSRegion: '',
  InvoiceFileOSSAccessKeyId: '',
  InvoiceFileOSSAccessKeySecret: '',
  InvoiceFileOSSCustomDomain: '',
  InvoiceFileS3Endpoint: '',
  InvoiceFileS3Bucket: '',
  InvoiceFileS3Region: '',
  InvoiceFileS3AccessKeyId: '',
  InvoiceFileS3AccessKeySecret: '',
  InvoiceFileS3CustomDomain: '',
  InvoiceFileCOSEndpoint: '',
  InvoiceFileCOSBucket: '',
  InvoiceFileCOSRegion: '',
  InvoiceFileCOSSecretId: '',
  InvoiceFileCOSSecretKey: '',
  InvoiceFileCOSCustomDomain: '',
  WorkerUrl: '',
  WorkerValidKey: '',
  WorkerAllowHttpImageRequestEnabled: false,
  LogConsumeEnabled: false,
  'performance_setting.disk_cache_enabled': false,
  'performance_setting.disk_cache_threshold_mb': 10,
  'performance_setting.disk_cache_max_size_mb': 1024,
  'performance_setting.disk_cache_path': '',
  'performance_setting.monitor_enabled': false,
  'performance_setting.monitor_cpu_threshold': 90,
  'performance_setting.monitor_memory_threshold': 90,
  'performance_setting.monitor_disk_threshold': 95,
  'perf_metrics_setting.enabled': true,
  'perf_metrics_setting.flush_interval': 5,
  'perf_metrics_setting.bucket_time': 'hour',
  'perf_metrics_setting.retention_days': 0,
}

export function OperationsSettings() {
  const { status } = useStatus()

  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/operations/$section'
      defaultSettings={defaultOperationsSettings}
      defaultSection={OPERATIONS_DEFAULT_SECTION}
      getSectionContent={getOperationsSectionContent}
      getSectionMeta={getOperationsSectionMeta}
      extraArgs={[
        status?.version as string | undefined,
        status?.start_time as number | null | undefined,
      ]}
      loadingMessage='Loading maintenance settings...'
    />
  )
}
