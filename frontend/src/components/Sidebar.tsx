import { NavLink } from "react-router-dom";
import styles from "./Sidebar.module.css";

const Sidebar: React.FC = () => {
  return (
    <aside className={styles.sidebar}>
      <div className={styles.sidebarTop}>
        {/*<div className={styles.logo}>fin.</div> */}
      </div>

      <nav className={styles.nav}>
        <NavLink
          to="/"
          end
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ""}`
          }
        >
          Dashboard
        </NavLink>

        {/* <NavLink
          to="/payment"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ""}`
          }
        >
          Payment
        </NavLink> */}

        <NavLink
          to="/audit-logs"
          className={({ isActive }) =>
            `${styles.navItem} ${isActive ? styles.active : ""}`
          }
        >
          Audit Logs
        </NavLink>
      </nav>
    </aside>
  );
};

export default Sidebar;