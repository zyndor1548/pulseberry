import { Routes } from "react-router-dom";
import Sidebar from "../../components/Sidebar";
import styles from "./Dashboard.module.css";

const Dashboard: React.FC = () => {
  return (
    <div className={styles.layout}>
      <Sidebar />
      
      <main className={styles.content}>
        <Routes>
        </Routes>
      </main>
    </div>
  );
};

export default Dashboard;