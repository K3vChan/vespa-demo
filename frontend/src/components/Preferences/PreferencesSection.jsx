import { ALL_GENRES, ALL_TAGS } from '../../constants';
import PillGroup from './PillGroup';
import SavePreferences from './SavePreferences';
import styles from './Preferences.module.css';

export default function PreferencesSection() {
  return (
    <div className={styles.prefsSection}>
      <h2>Preferences</h2>
      <hr className={styles.separator} />
      <PillGroup label="Genres" items={ALL_GENRES} type="genre" />
      <PillGroup label="Tags" items={ALL_TAGS} type="tag" />
      <hr className={styles.separator} />
      <SavePreferences />
    </div>
  );
}
