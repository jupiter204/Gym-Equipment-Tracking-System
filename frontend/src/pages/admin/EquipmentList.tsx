import React, { useEffect, useState } from 'react';
import axios from 'axios';
import apiClient from '../../services/apiClient';
import { Card, CardContent } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Plus, Search, MapPin, Tag, X, ChevronLeft, ChevronRight, AlertCircle, CheckCircle2 } from 'lucide-react';
import type { Equipment } from '../../types';

const EquipmentList: React.FC = () => {
  const [equipments, setEquipments] = useState<Equipment[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  
  // 分頁狀態
  const [page, setPage] = useState(1);
  const pageSize = 20;
  const [totalCount, setTotalCount] = useState(0);

  // 訊息反饋狀態 (取代 alert)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // 新增設備用的狀態
  const [showAddModal, setShowAddModal] = useState(false);
  const [formData, setFormData] = useState({
    asset_code: '',
    name: '',
    category: '',
    location: '',
    maint_interval: 30,
  });

  // 編輯設備用的狀態
  const [showEditModal, setShowEditModal] = useState(false);
  const [editFormData, setEditFormData] = useState({
    lid: '',
    asset_code: '',
    name: '',
    category: '',
    location: '',
    maint_interval: 30,
  });

  // QR Code 視窗狀態
  const [showQrModal, setShowQrModal] = useState(false);
  const [qrData, setQrData] = useState({ name: '', code: '' });

  const [isSubmitting, setIsSubmitting] = useState(false);

  // 取得真實的 API 資料
  const fetchEquipments = async (targetPage = page) => {
    setLoading(true);
    try {
      const offset = (targetPage - 1) * pageSize;
      const res = await apiClient.get<Equipment[]>('/private/equipments', {
        params: { limit: pageSize, offset },
      });
      setEquipments(res.data || []);
      const countHeader = res.headers['x-total-count'];
      if (countHeader) {
        setTotalCount(parseInt(countHeader, 10));
      }
    } catch {
      setMessage({ type: 'error', text: '無法取得設備列表，請確認是否已經登入！' });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let ignore = false;
    const load = async () => {
      try {
        const offset = (page - 1) * pageSize;
        const res = await apiClient.get<Equipment[]>('/private/equipments', {
          params: { limit: pageSize, offset },
        });
        if (!ignore) {
          setEquipments(res.data || []);
          const countHeader = res.headers['x-total-count'];
          if (countHeader) {
            setTotalCount(parseInt(countHeader, 10));
          }
        }
      } catch {
        if (!ignore) {
          setMessage({ type: 'error', text: '無法取得設備列表，請確認是否已經登入！' });
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

  // 處理新增表單變更
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  // 處理編輯表單變更
  const handleEditInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setEditFormData((prev) => ({ ...prev, [name]: value }));
  };

  // 打開編輯視窗
  const handleEditClick = (eq: Equipment) => {
    setEditFormData({
      lid: eq.lid,
      asset_code: eq.asset_code || '',
      name: eq.name || '',
      category: eq.category || '',
      location: eq.location || '',
      maint_interval: eq.maint_interval || 30,
    });
    setShowEditModal(true);
  };

  // 送出新增請求
  const handleAddSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (formData.maint_interval < 1) {
      setMessage({ type: 'error', text: '保養週期必須至少為 1 天！' });
      return;
    }
    setIsSubmitting(true);
    try {
      await apiClient.post('/private/equipment', {
        asset_code: formData.asset_code.trim(),
        name: formData.name.trim(),
        category: formData.category.trim(),
        location: formData.location.trim(),
        maint_interval: Number(formData.maint_interval),
      });
      setShowAddModal(false);
      setFormData({ asset_code: '', name: '', category: '', location: '', maint_interval: 30 });
      setMessage({ type: 'success', text: '設備新增成功！' });
      fetchEquipments(1);
      setPage(1);
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        setMessage({ type: 'error', text: err.response?.data?.error || '新增失敗，請確認編號是否重複或格式錯誤！' });
      } else {
        setMessage({ type: 'error', text: '新增失敗！' });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  // 送出編輯請求
  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (editFormData.maint_interval < 1) {
      setMessage({ type: 'error', text: '保養週期必須至少為 1 天！' });
      return;
    }
    setIsSubmitting(true);
    try {
      await apiClient.patch('/private/equipment', {
        lid: editFormData.lid,
        asset_code: editFormData.asset_code.trim(),
        name: editFormData.name.trim(),
        category: editFormData.category.trim(),
        location: editFormData.location.trim(),
        maint_interval: Number(editFormData.maint_interval),
      });
      setShowEditModal(false);
      setMessage({ type: 'success', text: '設備資料修改成功！' });
      fetchEquipments(page);
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        setMessage({ type: 'error', text: err.response?.data?.error || '修改失敗，請確認資料是否正確！' });
      } else {
        setMessage({ type: 'error', text: '修改失敗！' });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  // 下架/刪除設備
  const handleDelete = async (lid: string) => {
    if (!window.confirm('確定要下架此設備嗎？（若該設備已有維修紀錄，將進行下架封存以保留歷史稽核紀錄；若無紀錄則直接刪除）')) return;
    try {
      const res = await apiClient.delete('/private/equipment', { data: { lid } });
      setMessage({ type: 'success', text: res.data?.message || '設備已成功處理！' });
      fetchEquipments(page);
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        setMessage({ type: 'error', text: err.response?.data?.error || '操作失敗！' });
      } else {
        setMessage({ type: 'error', text: '操作失敗！' });
      }
    }
  };

  // 打開 QR Code 視窗
  const handleQrClick = (eq: Equipment) => {
    setQrData({ name: eq.name, code: eq.asset_code });
    setShowQrModal(true);
  };

  const getStatusBadge = (status: string) => {
    if (status === 'normal') {
      return <span className="px-2 py-1 bg-green-500/20 text-green-500 rounded-full text-xs whitespace-nowrap">狀態正常</span>;
    } else if (status === 'pending_maint') {
      return <span className="px-2 py-1 bg-amber-500/20 text-amber-500 rounded-full text-xs whitespace-nowrap">待保養</span>;
    } else if (status === 'repairing') {
      return <span className="px-2 py-1 bg-blue-500/20 text-blue-500 rounded-full text-xs whitespace-nowrap">檢修中</span>;
    } else if (status === 'faulty') {
      return <span className="px-2 py-1 bg-destructive/20 text-destructive rounded-full text-xs whitespace-nowrap">故障待修</span>;
    } else {
      return <span className="px-2 py-1 bg-secondary text-secondary-foreground rounded-full text-xs whitespace-nowrap">未知狀態</span>;
    }
  };

  const filteredEquipments = equipments.filter(
    (eq) =>
      eq.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      eq.asset_code.toLowerCase().includes(searchTerm.toLowerCase()) ||
      (eq.location && eq.location.toLowerCase().includes(searchTerm.toLowerCase()))
  );

  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize));

  return (
    <div className="space-y-6 relative">
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">器材管理列表</h1>
          <p className="text-muted-foreground mt-2">檢視所有健身器材的狀態、位置與保養週期 (共 {totalCount} 台)</p>
        </div>
        <Button className="flex items-center gap-2" onClick={() => setShowAddModal(true)}>
          <Plus className="w-4 h-4" /> 新增器材
        </Button>
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

      <div className="flex gap-4 items-center">
        <div className="relative flex-1 max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input 
            type="text" 
            placeholder="搜尋當前頁面器材名稱、編號或位置..." 
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full bg-card border border-border rounded-lg pl-10 pr-4 py-2 focus:outline-none focus:ring-2 focus:ring-primary text-foreground"
          />
        </div>
      </div>

      {loading ? (
        <div className="text-muted-foreground p-8 text-center">正在讀取器材資料，請稍候...</div>
      ) : (
        <div className="grid gap-4">
          {filteredEquipments.length === 0 ? (
            <div className="text-center p-8 text-muted-foreground border rounded-lg border-dashed">
              {searchTerm ? '找不到符合搜尋條件的器材。' : '目前還沒有任何設備資料。'}
            </div>
          ) : (
            filteredEquipments.map((eq) => (
              <Card key={eq.lid}>
                <CardContent className="p-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                  <div className="flex-1 space-y-2">
                    <div className="flex items-center gap-3">
                      <span className="font-bold text-lg">{eq.name}</span>
                      {getStatusBadge(eq.status)}
                    </div>
                    <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
                      <div className="flex items-center gap-1">
                        <Tag className="w-3 h-3" /> 編號: {eq.asset_code}
                      </div>
                      <div className="flex items-center gap-1">
                        <MapPin className="w-3 h-3" /> 位置: {eq.location || '未指定'}
                      </div>
                      <div>分類: {eq.category || '未分類'}</div>
                      <div>保養間隔: {eq.maint_interval} 天</div>
                      <div>上次保養: {eq.last_maint_date || '無紀錄'}</div>
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 mt-2 sm:mt-0">
                    <Button variant="outline" size="sm" onClick={() => handleEditClick(eq)}>
                      編輯資料
                    </Button>
                    <Button variant="secondary" size="sm" onClick={() => handleQrClick(eq)}>
                      QR Code
                    </Button>
                    <Button variant="destructive" size="sm" onClick={() => handleDelete(eq.lid)}>
                      下架
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))
          )}
        </div>
      )}

      {/* 分頁控制列 */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between pt-4 border-t border-border">
          <p className="text-sm text-muted-foreground">
            第 {page} 頁 / 共 {totalPages} 頁 (每頁 {pageSize} 筆，總計 {totalCount} 筆)
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

      {/* 新增設備的彈出視窗 */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-card w-full max-w-md rounded-xl shadow-xl overflow-hidden border border-border">
            <div className="flex justify-between items-center p-4 border-b border-border">
              <h2 className="text-xl font-bold">新增健身器材</h2>
              <button onClick={() => setShowAddModal(false)} className="text-muted-foreground hover:text-foreground">
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleAddSubmit} className="p-4 space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1.5">資產編號 (必填)</label>
                <input
                  required
                  type="text"
                  name="asset_code"
                  value={formData.asset_code}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="例如: RUN-003"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">器材名稱 (必填)</label>
                <input
                  required
                  type="text"
                  name="name"
                  value={formData.name}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="例如: 電動跑步機"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">分類</label>
                <input
                  type="text"
                  name="category"
                  value={formData.category}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="例如: 有氧器材"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">放置位置</label>
                <input
                  type="text"
                  name="location"
                  value={formData.location}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="例如: 1F 有氧區 A03"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">保養週期 (天數，至少 1 天)</label>
                <input
                  required
                  min={1}
                  type="number"
                  name="maint_interval"
                  value={formData.maint_interval}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                />
              </div>
              <div className="pt-4 flex justify-end gap-2">
                <Button type="button" variant="outline" onClick={() => setShowAddModal(false)}>
                  取消
                </Button>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? '處理中...' : '確認新增'}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 編輯設備的彈出視窗 */}
      {showEditModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-card w-full max-w-md rounded-xl shadow-xl overflow-hidden border border-border">
            <div className="flex justify-between items-center p-4 border-b border-border">
              <h2 className="text-xl font-bold">編輯健身器材</h2>
              <button onClick={() => setShowEditModal(false)} className="text-muted-foreground hover:text-foreground">
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleEditSubmit} className="p-4 space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1.5">資產編號</label>
                <input
                  required
                  type="text"
                  name="asset_code"
                  value={editFormData.asset_code}
                  onChange={handleEditInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">器材名稱</label>
                <input
                  required
                  type="text"
                  name="name"
                  value={editFormData.name}
                  onChange={handleEditInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">分類</label>
                <input
                  type="text"
                  name="category"
                  value={editFormData.category}
                  onChange={handleEditInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">放置位置</label>
                <input
                  type="text"
                  name="location"
                  value={editFormData.location}
                  onChange={handleEditInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">保養週期 (天數，至少 1 天)</label>
                <input
                  required
                  min={1}
                  type="number"
                  name="maint_interval"
                  value={editFormData.maint_interval}
                  onChange={handleEditInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                />
              </div>
              <div className="pt-4 flex justify-end gap-2">
                <Button type="button" variant="outline" onClick={() => setShowEditModal(false)}>
                  取消
                </Button>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? '處理中...' : '儲存修改'}
                </Button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* QR Code 彈出視窗 */}
      {showQrModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-card w-full max-w-sm rounded-xl shadow-xl overflow-hidden border border-border p-6 flex flex-col items-center">
            <h3 className="font-bold text-lg mb-2">{qrData.name}</h3>
            <p className="text-sm text-muted-foreground mb-4">編號: {qrData.code}</p>
            <div className="p-4 bg-white rounded-lg mb-4">
              <img
                src={`https://api.qrserver.com/v1/create-qr-code/?size=180x180&data=${encodeURIComponent(
                  window.location.origin + '/report/' + qrData.code
                )}`}
                alt="QR Code"
                className="w-44 h-44"
              />
            </div>
            <p className="text-xs text-muted-foreground text-center mb-4">
              掃描上方 QR Code 可直接進入此器材的民眾報修頁面。
            </p>
            <Button className="w-full" onClick={() => setShowQrModal(false)}>
              關閉
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};

export default EquipmentList;
