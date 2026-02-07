import Pill from './Pill';
import styles from './Preferences.module.css';

export default function PillGroup({ label, items, type }) {
  return (
    <div className={styles.prefGroup}>
      <span className={styles.groupLabel}>{label}</span>
      <div className={styles.pills}>
        {items.map((val) => (
          <Pill key={val} type={type} value={val} />
        ))}
      </div>
    </div>
  );
}
