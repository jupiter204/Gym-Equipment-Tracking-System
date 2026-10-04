import React, { useEffect, useState } from 'react';
import axios from 'axios';
import apiClient, { getStoredUser } from '../../services/apiClient';
import { Card, CardContent } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Plus, Users, Shield, User as UserIcon, X, Trash2, ChevronLeft, ChevronRight, CheckCircle2, AlertCircle } from 'lucide-react';
import type { User } from '../../types';

const UserManagement: React.FC = () => {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  
  // 分頁狀態
  const [page, setPage] = useState(1);
  const pageSize = 20;
  const [totalCount, setTotalCount] = useState(0);

  // 反饋訊息 (取代 alert)
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const currentUser = getStoredUser();

  // 新增使用者的狀態
  const [showAddModal, setShowAddModal] = useState(false);
  const [formData, setFormData] = useState({
    username: '',
    password: '',
    name: '',
    role: 'staff' as 'admin' | 'staff',
  });

  // 編輯使用者的狀態
  const [showEditModal, setShowEditModal] = useState(false);
  const [editFormData, setEditFormData] = useState({
    lid: '',
    name: '',
    role: 'staff' as 'admin' | 'staff',
    password: '',
  });

  const [isSubmitting, setIsSubmitting] = useState(false);

  // 取得使用者列表
  const fetchUsers = async (targetPage = page) => {
    setLoading(true);
    try {
      const offset = (targetPage - 1) * pageSize;
      const res = await apiClient.get<User[]>('/private/users', {
        params: { limit: pageSize, offset },
      });
      setUsers(res.data || []);
      const countHeader = res.headers['x-total-count'];
      if (countHeader) {
        setTotalCount(parseInt(countHeader, 10));
      }
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        setFeedback({ type: 'error', text: err.response?.data?.error || '無法取得使用者資料，可能沒有權限！' });
      } else {
        setFeedback({ type: 'error', text: '無法取得使用者資料' });
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let ignore = false;
    const load = async () => {
      try {
        const offset = (page - 1) * pageSize;
        const res = await apiClient.get<User[]>('/private/users', {
          params: { limit: pageSize, offset },
        });
        if (!ignore) {
          setUsers(res.data || []);
          const countHeader = res.headers['x-total-count'];
          if (countHeader) {
            setTotalCount(parseInt(countHeader, 10));
          }
        }
      } catch (err: unknown) {
        if (!ignore) {
          if (axios.isAxiosError(err)) {
            setFeedback({ type: 'error', text: err.response?.data?.error || '無法取得使用者資料，可能沒有權限！' });
          } else {
            setFeedback({ type: 'error', text: '無法取得使用者資料' });
          }
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
  }, [page, pageSize]);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  const handleEditInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target;
    setEditFormData((prev) => ({ ...prev, [name]: value }));
  };

  const handleAddSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (formData.password.length < 8) {
      setFeedback({ type: 'error', text: '密碼長度至少需 8 個字元！' });
      return;
    }
    if (new TextEncoder().encode(formData.password).length > 72) {
      setFeedback({ type: 'error', text: '密碼長度不可超過 72 位元組！' });
      return;
    }
    setIsSubmitting(true);
    try {
      await apiClient.post('/private/user', formData);
      setShowAddModal(false);
      setFormData({ username: '', password: '', name: '', role: 'staff' });
      setFeedback({ type: 'success', text: '帳號新增成功！' });
      fetchUsers(1);
      setPage(1);
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        setFeedback({ type: 'error', text: err.response?.data?.error || '新增失敗，請確認帳號是否重複！' });
      } else {
        setFeedback({ type: 'error', text: '新增失敗！' });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleEditClick = (user: User) => {
    setEditFormData({
      lid: user.lid,
      name: user.name,
      role: user.role,
      password: '',
    });
    setShowEditModal(true);
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (editFormData.password && editFormData.password.length < 8) {
      setFeedback({ type: 'error', text: '若要重設密碼，密碼長度至少需 8 個字元！' });
      return;
    }
    if (editFormData.password && new TextEncoder().encode(editFormData.password).length > 72) {
      setFeedback({ type: 'error', text: '密碼長度不可超過 72 位元組！' });
      return;
    }
    setIsSubmitting(true);
    try {
      const payload: { lid: string; name: string; role: 'admin' | 'staff'; password?: string } = {
        lid: editFormData.lid,
        name: editFormData.name,
        role: editFormData.role,
      };
      if (editFormData.password.trim()) {
        payload.password = editFormData.password.trim();
      }
      await apiClient.patch('/private/user', payload);
      setShowEditModal(false);
      setFeedback({ type: 'success', text: '使用者資料修改成功！' });
      fetchUsers(page);
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        setFeedback({ type: 'error', text: err.response?.data?.error || '修改失敗！' });
      } else {
        setFeedback({ type: 'error', text: '修改失敗！' });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDelete = async (lid: string) => {
    if (!window.confirm('確定要刪除這個使用者帳號嗎？')) return;
    try {
      await apiClient.delete('/private/user', { data: { lid } });
      setFeedback({ type: 'success', text: '帳號刪除成功！' });
      fetchUsers(page);
    } catch (err: unknown) {
      if (axios.isAxiosError(err)) {
        setFeedback({ type: 'error', text: err.response?.data?.error || '刪除失敗！' });
      } else {
        setFeedback({ type: 'error', text: '刪除失敗！' });
      }
    }
  };

  const adminCount = users.filter((u) => u.role === 'admin').length;
  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize));

  return (
    <div className="space-y-6 relative">
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight flex items-center gap-2">
            <Users className="w-8 h-8 text-primary" /> 人員管理
          </h1>
          <p className="text-muted-foreground mt-2">管理系統的維修人員與管理員帳號 (共 {totalCount} 位)</p>
        </div>
        <Button className="flex items-center gap-2" onClick={() => setShowAddModal(true)}>
          <Plus className="w-4 h-4" /> 新增帳號
        </Button>
      </div>

      {feedback && (
        <div
          className={`p-4 rounded-md flex items-center justify-between border ${
            feedback.type === 'success'
              ? 'bg-green-500/10 border-green-500/30 text-green-500'
              : 'bg-destructive/10 border-destructive/30 text-destructive'
          }`}
        >
          <div className="flex items-center gap-2">
            {feedback.type === 'success' ? <CheckCircle2 className="w-5 h-5" /> : <AlertCircle className="w-5 h-5" />}
            <span>{feedback.text}</span>
          </div>
          <button onClick={() => setFeedback(null)} className="text-muted-foreground hover:text-foreground">
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      {loading ? (
        <div className="p-8 text-muted-foreground text-center">正在讀取使用者資料，請稍候...</div>
      ) : (
        <div className="grid gap-4">
          {users.length === 0 ? (
            <div className="text-center p-8 text-muted-foreground border rounded-lg border-dashed">
              沒有任何使用者資料。
            </div>
          ) : (
            users.map((user) => {
              const isSelf = currentUser?.userUUID === user.lid;
              const isLastAdmin = user.role === 'admin' && adminCount <= 1;
              const canDelete = !isSelf && !isLastAdmin;

              return (
                <Card key={user.lid}>
                  <CardContent className="p-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                    <div className="flex-1 space-y-2">
                      <div className="flex items-center gap-3">
                        <span className="font-bold text-lg">{user.name}</span>
                        {user.role === 'admin' ? (
                          <span className="px-2 py-1 bg-primary/20 text-primary rounded-full text-xs whitespace-nowrap flex items-center gap-1">
                            <Shield className="w-3 h-3" /> 管理員
                          </span>
                        ) : (
                          <span className="px-2 py-1 bg-secondary text-secondary-foreground rounded-full text-xs whitespace-nowrap flex items-center gap-1">
                            <UserIcon className="w-3 h-3" /> 維修人員
                          </span>
                        )}
                        {isSelf && (
                          <span className="px-2 py-0.5 bg-blue-500/10 text-blue-400 border border-blue-500/20 rounded text-xs">
                            (目前帳號)
                          </span>
                        )}
                      </div>
                      <div className="text-sm text-muted-foreground flex items-center gap-4">
                        <div>
                          登入帳號: <span className="text-foreground font-medium">{user.username}</span>
                        </div>
                      </div>
                    </div>
                    <div className="flex flex-wrap items-center gap-2 mt-2 sm:mt-0">
                      <Button variant="outline" size="sm" onClick={() => handleEditClick(user)}>
                        編輯資料
                      </Button>
                      {canDelete && (
                        <Button variant="destructive" size="sm" onClick={() => handleDelete(user.lid)}>
                          <Trash2 className="w-4 h-4 mr-1" /> 刪除
                        </Button>
                      )}
                    </div>
                  </CardContent>
                </Card>
              );
            })
          )}
        </div>
      )}

      {/* 分頁控制列 */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between pt-4 border-t border-border">
          <p className="text-sm text-muted-foreground">
            第 {page} 頁 / 共 {totalPages} 頁 (每頁 {pageSize} 筆，總計 {totalCount} 位)
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

      {/* 新增使用者彈出視窗 */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-card w-full max-w-md rounded-xl shadow-xl overflow-hidden border border-border">
            <div className="flex justify-between items-center p-4 border-b border-border">
              <h2 className="text-xl font-bold">新增帳號</h2>
              <button onClick={() => setShowAddModal(false)} className="text-muted-foreground hover:text-foreground">
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleAddSubmit} className="p-4 space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1.5">登入帳號 (必填，3-32 字元)</label>
                <input
                  required
                  minLength={3}
                  maxLength={32}
                  type="text"
                  name="username"
                  value={formData.username}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="例如: staff01"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">密碼 (必填，至少 8 字元)</label>
                <input
                  required
                  minLength={8}
                  maxLength={72}
                  type="password"
                  name="password"
                  value={formData.password}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="請設定至少 8 位密碼"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">姓名 (必填)</label>
                <input
                  required
                  type="text"
                  name="name"
                  value={formData.name}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="例如: 王小明"
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">系統權限身分</label>
                <select
                  name="role"
                  value={formData.role}
                  onChange={handleInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                >
                  <option value="staff">維修專員 (一般權限)</option>
                  <option value="admin">系統管理員 (全權限)</option>
                </select>
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

      {/* 編輯使用者彈出視窗 */}
      {showEditModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
          <div className="bg-card w-full max-w-md rounded-xl shadow-xl overflow-hidden border border-border">
            <div className="flex justify-between items-center p-4 border-b border-border">
              <h2 className="text-xl font-bold">編輯人員資料</h2>
              <button onClick={() => setShowEditModal(false)} className="text-muted-foreground hover:text-foreground">
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleEditSubmit} className="p-4 space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1.5">姓名</label>
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
                <label className="block text-sm font-medium mb-1.5">重設密碼 (若不修改請留空)</label>
                <input
                  type="password"
                  name="password"
                  value={editFormData.password}
                  onChange={handleEditInputChange}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground"
                  placeholder="留空表示不變更密碼"
                  minLength={8}
                  maxLength={72}
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1.5">系統權限身分</label>
                <select
                  name="role"
                  value={editFormData.role}
                  onChange={handleEditInputChange}
                  disabled={currentUser?.userUUID === editFormData.lid}
                  className="w-full bg-input border border-border rounded-md px-3 py-2 text-foreground disabled:opacity-50"
                >
                  <option value="staff">維修專員 (一般權限)</option>
                  <option value="admin">系統管理員 (全權限)</option>
                </select>
                {currentUser?.userUUID === editFormData.lid && (
                  <p className="text-xs text-muted-foreground mt-1">無法在登入狀態下修改自己的角色。</p>
                )}
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
    </div>
  );
};

export default UserManagement;
