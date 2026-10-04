import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';

// 引入版面配置 (Layout) 與路由守衛
import AdminLayout from './components/layout/AdminLayout';
import RequireAuth from './components/auth/RequireAuth';

// 引入各個功能頁面
import ReportEquipment from './pages/public/ReportEquipment';
import Login from './pages/auth/Login';
import Dashboard from './pages/admin/Dashboard';
import EquipmentList from './pages/admin/EquipmentList';
import MaintenanceTasks from './pages/admin/MaintenanceTasks';
import Analytics from './pages/admin/Analytics';
import UserManagement from './pages/admin/UserManagement';

function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* 一般使用者用的路由：報修設備 */}
        <Route path="/report/:id" element={<ReportEquipment />} />
        
        {/* 登入頁面路由 */}
        <Route path="/login" element={<Login />} />
        
        {/* 後台路由守衛：必須登入才能進入 /admin */}
        <Route
          path="/admin"
          element={
            <RequireAuth allowedRoles={['admin', 'staff']}>
              <AdminLayout />
            </RequireAuth>
          }
        >
          {/* 預設首頁是儀表板 */}
          <Route index element={<Dashboard />} />
          {/* 設備列表頁面 */}
          <Route path="equipment" element={<EquipmentList />} />
          {/* 維修任務管理頁面 */}
          <Route path="maintenance" element={<MaintenanceTasks />} />
          {/* 數據分析頁面 */}
          <Route path="analytics" element={<Analytics />} />
          {/* 人員管理頁面 (僅限管理員) */}
          <Route
            path="users"
            element={
              <RequireAuth allowedRoles={['admin']}>
                <UserManagement />
              </RequireAuth>
            }
          />
        </Route>

        {/* 預設跳轉回登入頁面 */}
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
