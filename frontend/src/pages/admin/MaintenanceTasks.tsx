import React, { useEffect, useState } from 'react';
import axios from 'axios';
import apiClient from '../../services/apiClient';
import { Card, CardContent } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Wrench, Check, AlertTriangle, Clock } from 'lucide-react';
import type { MaintenanceRecord } from '../../types';

const MaintenanceTasks: React.FC = () => {
  const [tasks, setTasks] = useState<MaintenanceRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [resolvingId, setResolvingId] = useState<string | null>(null);

  const fetchTasks = async () => {
    setLoading(true);
    try {
      const res = await apiClient.get('/private/maintenance-records?resolved=false');
      setTasks(res.data || []);
    } catch {
      // Handled silently or empty state
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTasks();
  }, []);

  const handleResolve = async (lid: string) => {
    setResolvingId(lid);
    try {
      await apiClient.patch('/private/maintenance-records/resolve', {
        lid,
        resolve_note: '已完成修復與例行檢驗',
      });
      fetchTasks();
    } catch (error: unknown) {
      if (axios.isAxiosError(error)) {
        if (error.response?.status === 409) {
          alert('該任務已完成解決，畫面將自動更新。');
          fetchTasks();
        } else {
          alert(error.response?.data?.error || '標記失敗，請稍後再試！');
        }
      } else {
        alert('標記失敗，請檢查網路連線。');
      }
    } finally {
      setResolvingId(null);
    }
  };

  if (loading) {
    return <div className="text-muted-foreground p-8">正在努力載入任務清單...</div>;
  }

  return (
    <div className="space-y-6">
      <h1 className="text-3xl font-bold tracking-tight flex items-center gap-3">
        <Wrench className="w-8 h-8 text-primary" />
        待處理的維修任務
      </h1>

      {tasks.length === 0 ? (
        <Card className="border-dashed bg-secondary/50">
          <CardContent className="flex flex-col items-center justify-center p-12 text-center">
            <Check className="w-12 h-12 text-green-500 mb-4" />
            <p className="text-lg font-medium">太棒了！目前沒有待處理的任務</p>
            <p className="text-muted-foreground">所有的健身器材目前都非常健康！</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4">
          {tasks.map((task) => {
            const isSystemMaint = task.reporter_type === 'system' || task.description.includes('【系統自動偵測】');

            return (
              <Card
                key={task.lid}
                className={
                  isSystemMaint
                    ? 'border-amber-500/30 relative overflow-hidden'
                    : 'border-destructive/30 relative overflow-hidden'
                }
              >
                <div
                  className={`absolute top-0 left-0 w-1.5 h-full ${
                    isSystemMaint ? 'bg-amber-500' : 'bg-destructive'
                  }`}
                />
                <CardContent className="p-6">
                  <div className="flex flex-col md:flex-row justify-between gap-6">
                    <div className="flex-1 space-y-2">
                      <div className="flex items-center gap-2">
                        {isSystemMaint ? (
                          <Clock className="w-5 h-5 text-amber-500" />
                        ) : (
                          <AlertTriangle className="w-5 h-5 text-destructive" />
                        )}
                        <h3 className="font-semibold text-lg">
                          {isSystemMaint ? '定期保養單' : '故障通報單'} - 器材: {task.equipment_name || task.asset_code}
                        </h3>
                      </div>
                      <p className="text-muted-foreground bg-secondary/50 p-3 rounded-md">
                        問題描述：{task.description}
                      </p>
                      <div className="text-sm text-muted-foreground flex flex-wrap gap-4">
                        <span>通報時間: {new Date(task.created_at).toLocaleString()}</span>
                        <span>
                          通報人身分:{' '}
                          {task.reporter_type === 'system'
                            ? '系統自動排程'
                            : task.reporter_type === 'public'
                            ? '一般民眾'
                            : '健身房專員'}
                        </span>
                      </div>
                    </div>
                    <div className="flex items-end">
                      <Button
                        onClick={() => handleResolve(task.lid)}
                        disabled={resolvingId === task.lid}
                        className="bg-green-600 hover:bg-green-700 text-white flex items-center gap-2"
                      >
                        <Check className="w-4 h-4" />
                        {resolvingId === task.lid ? '處理中...' : '標記完成修復'}
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
};

export default MaintenanceTasks;
