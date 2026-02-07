import { createContext, useContext, useReducer } from 'react';

const AppContext = createContext(null);
const DispatchContext = createContext(null);

const initialState = {
  users: [],
  currentUserId: null,
  prefState: {},       // { "genre:Action": "like", "tag:classic": "dislike" }
  searchQuery: '',
  searchResults: null, // VespaResponse or null
  history: [],         // WatchHistoryEntry[]
  historyOpen: false,
  saveStatus: '',      // '' | 'saving' | 'saved'
  searchVersion: 0,
  recommendations: null, // VespaResponse or null
  recommendLoading: false,
};

function reducer(state, action) {
  switch (action.type) {
    case 'SET_USERS':
      return { ...state, users: action.payload };

    case 'SET_CURRENT_USER': {
      const userId = action.payload;
      const user = state.users.find((u) => u.id === userId);
      const prefState = {};
      if (user) {
        (user.preferences || []).forEach((p) => {
          prefState[`${p.type}:${p.value}`] = p.state;
        });
      }
      return { ...state, currentUserId: userId, prefState, history: [], historyOpen: false, recommendations: null };
    }

    case 'TOGGLE_PREF': {
      const { key, nextState } = action.payload;
      const prefState = { ...state.prefState };
      if (nextState === 'neutral') {
        delete prefState[key];
      } else {
        prefState[key] = nextState;
      }
      return { ...state, prefState };
    }

    case 'SET_SEARCH_QUERY':
      return { ...state, searchQuery: action.payload };

    case 'SET_SEARCH_RESULTS':
      return { ...state, searchResults: action.payload };

    case 'SET_HISTORY':
      return { ...state, history: action.payload };

    case 'TOGGLE_HISTORY':
      return { ...state, historyOpen: !state.historyOpen };

    case 'SET_SAVE_STATUS':
      return { ...state, saveStatus: action.payload };

    case 'UPDATE_USER_PREFS': {
      const { userId, preferences } = action.payload;
      const users = state.users.map((u) =>
        u.id === userId ? { ...u, preferences } : u
      );
      return { ...state, users };
    }

    case 'BUMP_SEARCH_VERSION':
      return { ...state, searchVersion: state.searchVersion + 1 };

    case 'SET_RECOMMENDATIONS':
      return { ...state, recommendations: action.payload };

    case 'SET_RECOMMEND_LOADING':
      return { ...state, recommendLoading: action.payload };

    default:
      return state;
  }
}

export function AppProvider({ children }) {
  const [state, dispatch] = useReducer(reducer, initialState);
  return (
    <AppContext.Provider value={state}>
      <DispatchContext.Provider value={dispatch}>
        {children}
      </DispatchContext.Provider>
    </AppContext.Provider>
  );
}

export function useAppState() {
  return useContext(AppContext);
}

export function useAppDispatch() {
  return useContext(DispatchContext);
}
