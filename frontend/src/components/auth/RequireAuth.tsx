import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { getStoredUser } from '../../services/apiClient';

interface RequireAuthProps {
  allowedRoles?: Array<'admin' | 'staff'>;
  children?: React.ReactNode;
}

export const RequireAuth: React.FC<RequireAuthProps> = ({ allowedRoles, children }) => {
  const location = useLocation();
  const token = localStorage.getItem('access_token');
  const user = getStoredUser();

  if (!token || !user) {
    // 尚未登入，導向登入頁並記住來源路徑
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  if (allowedRoles && !allowedRoles.includes(user.role)) {
    // 角色權限不足，導向後台首頁
    return <Navigate to="/admin" replace />;
  }

  return children ? <>{children}</> : null;
};

export default RequireAuth;
