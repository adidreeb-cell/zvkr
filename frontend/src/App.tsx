import Router, { Route, route } from "preact-router";
import { Layout } from "./components/ui/Layout";
import { LoginPage } from "./pages/Login";
import { Dashboard } from "./pages/Dashboard";
import { UploadPage } from "./pages/Upload";
import { DatasetView } from "./pages/DatasetView";
import { AdminUsers } from "./pages/AdminUsers";
import { AdvancedAnalytics } from "./pages/AdvancedAnalytics";
import { Settings } from "./pages/Settings";
import { SetupPage } from "./pages/Setup";

interface ProtectedRouteProps {
  component: any;
  path: string;
  [key: string]: any;
}

const ProtectedRoute = ({
  component: Component,
  ...rest
}: ProtectedRouteProps) => {
  const token = localStorage.getItem("token");
  if (!token) {
    // Делаем редирект асинхронно, чтобы не блокировать рендер
    setTimeout(() => route("/login", true), 0);
    return null;
  }
  return (
    <Layout>
      <Component {...rest} />
    </Layout>
  );
};

export function App() {
  return (
    <Router>
      <Route path="/login" component={LoginPage} />
      <ProtectedRoute path="/" component={Dashboard} />
      <ProtectedRoute path="/upload" component={UploadPage} />
      <ProtectedRoute path="/dataset/:id" component={DatasetView} />
      <ProtectedRoute path="/admin/users" component={AdminUsers} />
      <ProtectedRoute
        path="/analytics/advanced"
        component={AdvancedAnalytics}
      />
      <ProtectedRoute path="/settings" component={Settings} />
      <Route path="/setup" component={SetupPage} />
    </Router>
  );
}
