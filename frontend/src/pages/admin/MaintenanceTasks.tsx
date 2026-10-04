import React, { useEffect, useState } from 'react';
import axios from 'axios';
import apiClient from '../../services/apiClient';
import { Card, CardContent } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Wrench, Check, AlertTriangle, Clock, ChevronLeft, ChevronRight, X, AlertCircle, CheckCircle2 } from 'lucide-react';
import type { MaintenanceRecord } from '../../types';

const MaintenanceTasks: React.FC = () => {
  const [tasks, setTasks] = useState<MaintenanceRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [resolvingId, setResolvingId] = useState<string | null>(null);

  // 分頁狀態
  const [page, setPage] = useState(1);
  const pageSize = 20;
  const [totalCount, setTotalCount] = useState(0);

  // 反饋訊息狀態 (取代 alert)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const fetchTasks = async (targetPage = page) => {
    setLoading(true);
    try {
      const offset = (targetPage - 1) * pageSize;
      const res = await apiClient.get<MaintenanceRecord[]>('/private/maintenance-records', {
        params: { resolved: 'false', limit: pageSize, offset },
      });
      setTasks(res.data || []);
      const countHeader = res.headers['x-total-count'];
      if (countHeader) {
        setTotalCount(parseInt(countHeader, 10));
      }
    } catch {
      setMessage({ type: 'error', text: '載入任務清單失敗，請稍後重試。' });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let ignore = false;
    const load = async () => {
      try {
        const offset = (page - 1) * pageSize;
        const res = await apiClient.get<MaintenanceRecord[]>('/private/maintenance-records', {
          params: { resolved: 'false', limit: pageSize, offset },
        });
        if (!ignore) {
          setTasks(res.data || []);
          const countHeader = res.headers['x-total-count'];
          if (countHeader) {
            setTotalCount(parseInt(countHeader, 10));
          }
        }
      } catch {
        if (!ignore) {
          setMessage({ type: 'error', text: '載入任務清單失敗，請稍後重試。' });
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
  }, [page]);

  const handleResolve = async (lid: string) => {
    setResolvingId(lid);
    try {
      await apiClient.patch('/private/maintenance-records/resolve', {
        lid,
        resolve_note: '已完成修復與例行檢驗',
      });
      setMessage({ type: 'success', text: '已成功標記該任務為完成修復！' });
      fetchTasks(page);
    } catch (error: unknown) {
      if (axios.isAxiosError(error)) {
        if (error.response?.status === 409) {
          setMessage({ type: 'error', text: '該任務已完成解決，畫面將自動更新。' });
          fetchTasks(page);
        } else {
          setMessage({ type: 'error', text: error.response?.data?.error || '標記失敗，請稍後再試！' });
        }
      } else {
        setMessage({ type: 'error', text: '標記失敗，請檢查網路連線。' });
      }
    } finally {
      setResolvingId(null);
    }
  };

  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize));

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <h1 className="text-3xl font-bold tracking-tight flex items-center gap-3">
          <Wrench className="w-8 h-8 text-primary" />
          待處理的維修任務 ({totalCount})
        </h1>
      </div>

      {message && (
        <div
          className={`p-4 rounded-md flex items-center justify-between border ${
            message.type === 'success'
              ? 'bg-green-500/10 border-green-500/30 text-green-500'
              : 'bg-destructive/10 border-destructive/30 text-destructive'
          }`}
        >
          <div className="flex items-center gap-2">
            {message.type === 'success' ? <CheckCircle2 className="w-5 h-5" /> : <AlertCircle className="w-5 h-5" />}
            <span>{message.text}</span>
          </div>
          <button onClick={() => setMessage(null)} className="text-muted-foreground hover:text-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      {loading ? (
        <div className="text-muted-foreground p-8 text-center">正在努力載入任務清單...</div>
      ) : tasks.length === 0 ? (
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

      {/* 分頁控制列 */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between pt-4 border-t border-border">
          <p className="text-sm text-muted-foreground">
            第 {page} 頁 / 共 {totalPages} 頁 (每頁 {pageSize} 筆，未解決總計 {totalCount} 筆)
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1 || loading}
            >
              <ChevronLeft className="w-4 h-4 mr-1" /> 上一頁
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages || loading}
            >
              下一頁 <ChevronRight className="w-4 h-4 ml-1" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};

export default MaintenanceTasks;
