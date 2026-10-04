import React, { useEffect, useState } from 'react';
import apiClient from '../../services/apiClient';
import { Card, CardContent, CardHeader, CardTitle } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Activity, AlertTriangle, CheckCircle, Clock, AlertCircle } from 'lucide-react';
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip as RechartsTooltip, Legend } from 'recharts';
import type { StatsResponse, EquipmentSummary } from '../../types';

const Dashboard: React.FC = () => {
  const [stats, setStats] = useState<EquipmentSummary>({
    total: 0,
    normal: 0,
    faulty: 0,
    pending: 0,
    repairing: 0,
    faultRate: 0,
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchStats = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiClient.get<StatsResponse>('/private/stats');
      if (res.data?.equipment_summary) {
        setStats(res.data.equipment_summary);
      }
    } catch {
      setError('無法載入系統統計數據，請檢查網路連線或稍後再試。');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let ignore = false;
    const load = async () => {
      try {
        const res = await apiClient.get<StatsResponse>('/private/stats');
        if (!ignore && res.data?.equipment_summary) {
          setStats(res.data.equipment_summary);
        }
      } catch {
        if (!ignore) {
          setError('無法載入系統統計數據，請檢查網路連線或稍後再試。');
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

  const normalCount = stats.normal;

  const chartData = [
    { name: '正常運作', value: normalCount, color: '#22c55e' },
    { name: '待保養', value: stats.pending, color: '#f59e0b' },
    { name: '檢修中', value: stats.repairing, color: '#3b82f6' },
    { name: '故障待修', value: stats.faulty, color: '#ef4444' },
  ].filter((item) => item.value > 0);

  if (loading) {
    return <div className="p-8 text-muted-foreground">正在載入數據中...</div>;
  }

  if (error) {
    return (
      <div className="p-8 max-w-lg mx-auto mt-12 bg-destructive/10 border border-destructive/20 rounded-lg text-center space-y-4">
        <AlertCircle className="w-10 h-10 text-destructive mx-auto" />
        <h2 className="text-lg font-semibold text-destructive">載入失敗</h2>
        <p className="text-sm text-muted-foreground">{error}</p>
        <Button onClick={fetchStats} variant="outline">
          重新整理
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">系統儀表板</h1>
        <p className="text-muted-foreground mt-2">即時器材狀態概覽</p>
      </div>

      <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">總器材數</CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{stats.total}</div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">正常運作</CardTitle>
            <CheckCircle className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-500">{normalCount}</div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">待保養</CardTitle>
            <Clock className="h-4 w-4 text-amber-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-amber-500">{stats.pending}</div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium">故障與檢修</CardTitle>
            <AlertTriangle className="h-4 w-4 text-destructive" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-destructive">{stats.faulty + stats.repairing}</div>
            <p className="text-xs text-muted-foreground mt-1">異常比例：{stats.faultRate}%</p>
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        <Card className="col-span-1">
          <CardHeader>
            <CardTitle>器材狀態分佈圓餅圖</CardTitle>
          </CardHeader>
          <CardContent className="h-[300px]">
            {chartData.length === 0 ? (
              <div className="h-full flex items-center justify-center text-muted-foreground">
                目前沒有器材數據
              </div>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={chartData}
                    cx="50%"
                    cy="50%"
                    innerRadius={60}
                    outerRadius={80}
                    paddingAngle={5}
                    dataKey="value"
                    stroke="none"
                  >
                    {chartData.map((entry, index) => (
                      <Cell key={`cell-${index}`} fill={entry.color} />
                    ))}
                  </Pie>
                  <RechartsTooltip
                    contentStyle={{ backgroundColor: '#18181b', borderColor: '#27272a', color: '#fafafa' }}
                    itemStyle={{ color: '#fafafa' }}
                  />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default Dashboard;
