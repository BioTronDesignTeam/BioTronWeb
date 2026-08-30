import { Navigate, Routes, Route } from 'react-router-dom';
import SmoothScroll from './lib/SmoothScroll';
import Nav from './components/Nav';
import Footer from './components/Footer';
import Cursor from './components/Cursor';
import ScrollProgress from './components/ScrollProgress';
import PageTransition from './components/PageTransition';
import Home from './pages/Home';
import ProjectsIndex from './pages/ProjectsIndex';
import ProjectDetail from './pages/ProjectDetail';
import JoinPage from './pages/JoinPage';
import SponsorsPage from './pages/SponsorsPage';
import CalendarPage from './pages/CalendarPage';
import NotFound from './pages/NotFound';

export default function App() {
  return (
    <SmoothScroll>
      <div className="app grain">
        <a href="#main" className="skip-link">
          Skip to content
        </a>
        <Cursor />
        <ScrollProgress />
        <Nav />

        <PageTransition>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/projects" element={<ProjectsIndex />} />
            <Route path="/projects/:slug" element={<ProjectDetail />} />
            <Route path="/past-projects" element={<Navigate to="/projects#past" replace />} />
            <Route path="/join" element={<JoinPage />} />
            <Route path="/sponsors" element={<SponsorsPage />} />
            <Route path="/calendar" element={<CalendarPage />} />
            <Route path="*" element={<NotFound />} />
          </Routes>
        </PageTransition>

        <Footer />
      </div>
    </SmoothScroll>
  );
}
