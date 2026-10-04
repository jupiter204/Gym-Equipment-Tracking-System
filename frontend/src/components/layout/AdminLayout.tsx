import React from 'react';
import { Outlet, Link, useNavigate } from 'react-router-dom';
import { Activity, Dumbbell, ClipboardList, BarChart3, Users, LogOut } from 'lucide-react';
import apiClient, { getStoredUser } from '../../services/apiClient';

const AdminLayout: React.FC = () => {
  const navigate = useNavigate();
  const currentUser = getStoredUser();

  const handleLogout = async () => {
    const refreshToken = localStorage.getItem('refresh_token');
    try {
      if (refreshToken) {
        await apiClient.post('/auth/logout', { refresh_token: refreshToken });
      }
    } catch {
      // Ignore logout request errors
    } finally {
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
      navigate('/login');
    }
  };

  return (
    <div className="flex h-screen bg-background text-foreground">
      {/* 側邊導覽列 (Sidebar) */}
      <aside className="w-64 border-r border-border bg-card flex flex-col">
        {/* 系統標誌區 */}
        <div className="p-6 border-b border-border flex items-center gap-3">
          <Activity className="w-8 h-8 text-primary" />
          <span className="text-xl font-bold tracking-tight">GETS 系統</span>
        </div>
        
        {/* 選單列表 */}
        <nav className="flex-1 p-4 space-y-2">
          {/* 儀表板連結 */}
          <Link to="/admin" className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-secondary transition-colors">
            <Activity className="w-5 h-5 text-muted-foreground" />
            <span>首頁儀表板</span>
          </Link>
          {/* 器材管理連結 */}
          <Link to="/admin/equipment" className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-secondary transition-colors">
            <Dumbbell className="w-5 h-5 text-muted-foreground" />
            <span>器材管理</span>
          </Link>
          {/* 維修任務連結 */}
          <Link to="/admin/maintenance" className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-secondary transition-colors">
            <ClipboardList className="w-5 h-5 text-muted-foreground" />
            <span>維修任務</span>
          </Link>
          {/* 數據分析連結 */}
          <Link to="/admin/analytics" className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-secondary transition-colors">
            <BarChart3 className="w-5 h-5 text-muted-foreground" />
            <span>數據分析</span>
          </Link>
          {/* 人員管理連結 - 僅限管理員顯示 */}
          {currentUser?.role === 'admin' && (
            <Link to="/admin/users" className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-secondary transition-colors">
              <Users className="w-5 h-5 text-muted-foreground" />
              <span>人員管理</span>
            </Link>
          )}
        </nav>

        {/* 底部登出按鈕 */}
        <div className="p-4 border-t border-border">
          <button 
            onClick={handleLogout}
            className="flex items-center gap-3 px-3 py-2 w-full text-left rounded-md hover:bg-destructive/10 text-destructive transition-colors cursor-pointer"
          >
            <LogOut className="w-5 h-5" />
            <span>登出系統</span>
          </button>
        </div>
      </aside>

      {/* 右側主要內容區 */}
      <main className="flex-1 overflow-auto bg-background p-8">
        <Outlet />
      </main>
    </div>
  );
};

export default AdminLayout;
