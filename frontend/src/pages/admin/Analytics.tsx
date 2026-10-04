import React, { useEffect, useState } from 'react';
import apiClient from '../../services/apiClient';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Download, Loader2 } from 'lucide-react';
import { 
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip as RechartsTooltip, ResponsiveContainer,
  LineChart, Line
} from 'recharts';
import type { Equipment, MaintenanceRecord } from '../../types';

interface MonthTrend {
  name: string;
  faults: number;
  maintenance: number;
}

interface CategoryStat {
  name: string;
  count: number;
}

const Analytics: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [monthData, setMonthData] = useState<MonthTrend[]>([]);
  const [categoryData, setCategoryData] = useState<CategoryStat[]>([]);
  const [rawData, setRawData] = useState<MaintenanceRecord[]>([]);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [eqRes, maintRes] = await Promise.all([
          apiClient.get('/private/equipments'),
          apiClient.get('/private/maintenance-records'),
        ]);

        const equipments: Equipment[] = eqRes.data || [];
        const records: MaintenanceRecord[] = maintRes.data || [];
        setRawData(records);

        // 1. 計算各分類的維修次數
        const eqCategoryMap: Record<string, string> = {};
        equipments.forEach((eq) => {
          eqCategoryMap[eq.lid] = eq.category || '未分類';
        });

        const catMap: Record<string, number> = {};
        records.forEach((r) => {
          const cat = eqCategoryMap[r.equipment_id] || '未知分類';
          catMap[cat] = (catMap[cat] || 0) + 1;
        });

        const computedCategory = Object.keys(catMap).map((k) => ({ name: k, count: catMap[k] }));
        if (computedCategory.length === 0) {
          computedCategory.push({ name: '無故障紀錄', count: 0 });
        }
        setCategoryData(computedCategory);

        // 2. 計算近六個月的通報與保養趨勢
        const months = Array.from({ length: 6 }).map((_, i) => {
          const d = new Date();
          d.setMonth(d.getMonth() - (5 - i));
          return {
            monthKey: `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`,
            label: `${d.getMonth() + 1}月`,
            faults: 0,
            maintenance: 0,
          };
        });

        records.forEach((r) => {
          const d = new Date(r.created_at);
          const mKey = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
          const monthObj = months.find((m) => m.monthKey === mKey);
          if (monthObj) {
            if (r.is_resolved) {
              monthObj.maintenance += 1;
            } else {
              monthObj.faults += 1;
            }
          }
        });

        setMonthData(
          months.map((m) => ({ name: m.label, faults: m.faults, maintenance: m.maintenance }))
        );
      } catch {
        // Silently handled
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  const handleExportCSV = () => {
    if (rawData.length === 0) {
      alert('沒有資料可匯出');
      return;
    }

    const headers = ['通報單號', '設備名稱', '資產編號', '回報者類型', '問題描述', '處理狀態', '處理備註', '通報時間'];
    
    const csvContent = [
      headers.join(','),
      ...rawData.map((r) => [
        r.lid,
        `"${(r.equipment_name || '').replace(/"/g, '""')}"`,
        `"${(r.asset_code || '').replace(/"/g, '""')}"`,
        r.reporter_type === 'public' ? '民眾' : r.reporter_type === 'system' ? '系統' : '員工',
        `"${(r.description || '').replace(/"/g, '""')}"`,
        r.is_resolved ? '已解決' : '未解決',
        `"${(r.resolve_note || '').replace(/"/g, '""')}"`,
        new Date(r.created_at).toLocaleString(),
      ].join(',')),
    ].join('\n');

    const blob = new Blob(['\uFEFF' + csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.setAttribute('download', `維修紀錄報表_${Date.now()}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  if (loading) {
    return (
      <div className="flex h-[50vh] items-center justify-center text-muted-foreground">
        <Loader2 className="w-8 h-8 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">數據分析中心</h1>
          <p className="text-muted-foreground mt-2">檢視器材維修趨勢與匯出報表</p>
        </div>
        <Button variant="outline" className="flex items-center gap-2" onClick={handleExportCSV}>
          <Download className="w-4 h-4" /> 匯出 CSV 檔案
        </Button>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>近六個月維修與保養趨勢</CardTitle>
          </CardHeader>
          <CardContent className="h-[300px]">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={monthData} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#27272a" />
                <XAxis dataKey="name" stroke="#a1a1aa" />
                <YAxis stroke="#a1a1aa" />
                <RechartsTooltip 
                  contentStyle={{ backgroundColor: '#18181b', borderColor: '#27272a', color: '#fafafa' }}
                />
                <Line type="monotone" dataKey="faults" name="未解決通報" stroke="#ef4444" strokeWidth={2} />
                <Line type="monotone" dataKey="maintenance" name="已修復任務" stroke="#3b82f6" strokeWidth={2} />
              </LineChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>各分類歷史故障次數統計</CardTitle>
          </CardHeader>
          <CardContent className="h-[300px]">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={categoryData} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#27272a" />
                <XAxis dataKey="name" stroke="#a1a1aa" />
                <YAxis stroke="#a1a1aa" />
                <RechartsTooltip 
                  cursor={{ fill: '#27272a' }}
                  contentStyle={{ backgroundColor: '#18181b', borderColor: '#27272a', color: '#fafafa' }}
                />
                <Bar dataKey="count" name="故障通報總數" fill="#f59e0b" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default Analytics;
