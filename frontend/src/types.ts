export interface User {
  lid: string;
  id?: string;
  username: string;
  name: string;
  role: 'admin' | 'staff';
}

export interface Equipment {
  lid: string;
  id?: string;
  asset_code: string;
  name: string;
  category?: string;
  last_maint_date: string;
  maint_interval: number;
  status: 'normal' | 'pending_maint' | 'repairing' | 'faulty';
  location?: string;
  has_active_report?: boolean;
}

export interface MaintenanceRecord {
  lid: string;
  id?: string | number;
  equipment_id: string;
  equipment_name?: string;
  asset_code?: string;
  reporter_type: 'public' | 'staff' | 'system';
  description: string;
  is_resolved: boolean;
  resolve_note?: string;
  created_at: string;
}

export interface EquipmentSummary {
  total: number;
  normal: number;
  faulty: number;
  pending: number;
  repairing: number;
  faultRate: number;
}

export interface CategoryStat {
  name: string;
  count: number;
}

export interface MonthlyTrend {
  name: string;
  monthKey: string;
  faults: number;
  maintenance: number;
}

export interface StatsResponse {
  equipment_summary: EquipmentSummary;
  category_stats: CategoryStat[];
  monthly_trends: MonthlyTrend[];
}

