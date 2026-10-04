import React, { useEffect, useState } from 'react';
import apiClient from '../../services/apiClient';
import { csvCell } from '../../lib/csv';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Download, Loader2, AlertCircle } from 'lucide-react';
import { 
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip as RechartsTooltip, ResponsiveContainer,
  LineChart, Line
} from 'recharts';
import type { StatsResponse, MonthlyTrend, CategoryStat, MaintenanceRecord } from '../../types';

const Analytics: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [exporting, setExporting] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [monthData, setMonthData] = useState<MonthlyTrend[]>([]);
  const [categoryData, setCategoryData] = useState<CategoryStat[]>([]);

  useEffect(() => {
    let ignore = false;
    const load = async () => {
      try {
        const res = await apiClient.get<StatsResponse>('/private/stats');
        if (!ignore && res.data) {
          setMonthData(res.data.monthly_trends || []);
          setCategoryData(res.data.category_stats || []);
        }
      } catch {
        if (!ignore) {
          setErrorMsg('載入統計數據失敗，請稍後重試。');
        }
      } finally {
        if (!ignore) {
          setLoading(false);
        }
      }
    };
    void load();
    return () => {
      ignore = true;
    };
  }, []);

  const handleExportCSV = async () => {
    setExporting(true);
    setErrorMsg(null);
    try {
      // 透過分頁迴圈取得全量紀錄
      let allRecords: MaintenanceRecord[] = [];
      let offset = 0;
      const limit = 100;
      let hasMore = true;

      while (hasMore) {
        const res = await apiClient.get<MaintenanceRecord[]>('/private/maintenance-records', {
          params: { limit, offset },
        });
        const batch = res.data || [];
        allRecords = allRecords.concat(batch);
        const total = parseInt(res.headers['x-total-count'] || '0', 10);
        offset += batch.length;
        if (batch.length === 0 || (total > 0 && allRecords.length >= total)) {
          hasMore = false;
        }
      }

      if (allRecords.length === 0) {
        setErrorMsg('目前尚無任何維修紀錄可供匯出。');
        return;
      }

      const headers = ['通報單號', '設備名稱', '資產編號', '回報者類型', '問題描述', '處理狀態', '處理備註', '通報時間'];
      
      const csvContent = [
        headers.map((h) => csvCell(h)).join(','),
        ...allRecords.map((r) => [
          csvCell(r.lid),
          csvCell(r.equipment_name || ''),
          csvCell(r.asset_code || ''),
          csvCell(r.reporter_type === 'public' ? '民眾' : r.reporter_type === 'system' ? '系統' : '員工'),
          csvCell(r.description || ''),
          csvCell(r.is_resolved ? '已解決' : '未解決'),
          csvCell(r.resolve_note || ''),
          csvCell(new Date(r.created_at).toLocaleString()),
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
    } catch {
      setErrorMsg('匯出報表時發生錯誤，請稍後重試。');
    } finally {
      setExporting(false);
    }
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
        <Button variant="outline" className="flex items-center gap-2" onClick={handleExportCSV} disabled={exporting}>
          {exporting ? <Loader2 className="w-4 h-4 animate-spin" /> : <Download className="w-4 h-4" />}
          {exporting ? '正在匯出中...' : '匯出 CSV 檔案'}
        </Button>
      </div>

      {errorMsg && (
        <div className="p-4 bg-destructive/10 border border-destructive/20 text-destructive rounded-md flex items-center gap-2">
          <AlertCircle className="w-5 h-5 flex-shrink-0" />
          <span>{errorMsg}</span>
        </div>
      )}

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
