import React, { useState, useEffect, useRef } from 'react';
import { 
  Activity, 
  Database, 
  Users, 
  Clock, 
  Signal, 
  Terminal,
  Server as ServerIcon,
  RefreshCw,
  AlertCircle
} from 'lucide-react';
import { 
  LineChart, 
  Line, 
  AreaChart, 
  Area, 
  XAxis, 
  YAxis, 
  CartesianGrid, 
  Tooltip, 
  ResponsiveContainer 
} from 'recharts';

const NODES = [
  { id: 'node1', url: 'http://localhost:8081' },
  { id: 'node2', url: 'http://localhost:8082' },
  { id: 'node3', url: 'http://localhost:8083' },
];

const parseMetrics = (text) => {
  const metrics = {};
  const lines = text.split('\n');
  lines.forEach(line => {
    if (line.startsWith('kvstore_')) {
      const [nameAndLabels, value] = line.split(' ');
      const name = nameAndLabels.split('{')[0];
      const val = parseFloat(value);
      
      if (!metrics[name]) metrics[name] = [];
      metrics[name].push({ labels: nameAndLabels, value: val });
    }
  });
  return metrics;
};

const Dashboard = () => {
  const [metricsData, setMetricsData] = useState({});
  const [liveUpdates, setLiveUpdates] = useState([]);
  const [history, setHistory] = useState([]);
  const [activeNode, setActiveNode] = useState(NODES[0]);
  const sseRef = useRef(null);

  const fetchAllMetrics = async () => {
    const newData = {};
    for (const node of NODES) {
      try {
        const res = await fetch(`${node.url}/metrics`);
        const text = await res.text();
        newData[node.id] = parseMetrics(text);
      } catch (err) {
        newData[node.id] = { error: true };
      }
    }
    setMetricsData(newData);
    
    // Add to history for charts
    const timestamp = new Date().toLocaleTimeString();
    setHistory(prev => {
      const newPoint = { timestamp };
      NODES.forEach(node => {
        const localWrites = newData[node.id]?.kvstore_writes_total?.find(m => m.labels.includes('local'))?.value || 0;
        newPoint[`${node.id}_throughput`] = localWrites;
      });
      return [...prev.slice(-19), newPoint];
    });
  };

  useEffect(() => {
    const interval = setInterval(fetchAllMetrics, 2000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (sseRef.current) sseRef.current.close();
    
    // Watch a specific key for live updates (demo)
    const sse = new EventSource(`${activeNode.url}/watch/test-watch`);
    sse.onmessage = (event) => {
      const data = JSON.parse(event.data);
      setLiveUpdates(prev => [{ ...data, id: Date.now() }, ...prev.slice(0, 9)]);
    };
    sseRef.current = sse;
    
    return () => sse.close();
  }, [activeNode]);

  const getMetricValue = (nodeId, name, labelFilter) => {
    const m = metricsData[nodeId]?.[name];
    if (!m) return 0;
    if (!labelFilter) return m[0]?.value || 0;
    return m.find(item => item.labels.includes(labelFilter))?.value || 0;
  };

  return (
    <div className="min-h-screen p-6 space-y-6">
      {/* Header */}
      <header className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold gradient-text tracking-tight">KV-Store Dashboard</h1>
          <p className="text-gray-400">Distributed Consensus & Real-time Monitoring</p>
        </div>
        <div className="flex gap-3">
          {NODES.map(node => (
            <button
              key={node.id}
              onClick={() => setActiveNode(node)}
              className={`px-4 py-2 rounded-lg border transition-all flex items-center gap-2 ${
                activeNode.id === node.id 
                  ? 'bg-blue-500/20 border-blue-500 text-blue-400' 
                  : 'bg-card border-border text-gray-400 hover:border-gray-600'
              }`}
            >
              <ServerIcon size={16} />
              {node.id.toUpperCase()}
              <div className={`w-2 h-2 rounded-full ${metricsData[node.id]?.error ? 'bg-red-500' : 'bg-green-500'}`} />
            </button>
          ))}
        </div>
      </header>

      {/* Main Stats */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        <StatCard 
          icon={<Activity className="text-blue-400" />} 
          label="Total Local Writes" 
          value={getMetricValue(activeNode.id, 'kvstore_writes_total', 'local')}
        />
        <StatCard 
          icon={<RefreshCw className="text-purple-400" />} 
          label="Replicated Writes" 
          value={getMetricValue(activeNode.id, 'kvstore_writes_total', 'replicated')}
        />
        <StatCard 
          icon={<Users className="text-green-400" />} 
          label="Active Watchers" 
          value={getMetricValue(activeNode.id, 'kvstore_active_watchers')}
        />
        <StatCard 
          icon={<Signal className="text-yellow-400" />} 
          label="Gossip Rounds" 
          value={getMetricValue(activeNode.id, 'kvstore_gossip_rounds_total')}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Throughput Chart */}
        <div className="lg:col-span-2 glass p-6 h-[400px] flex flex-col">
          <div className="flex justify-between items-center mb-6">
            <h3 className="text-lg font-semibold flex items-center gap-2">
              <Signal size={20} className="text-blue-400" />
              Cluster Throughput
            </h3>
          </div>
          <div className="flex-1">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={history}>
                <CartesianGrid strokeDasharray="3 3" stroke="#27272a" vertical={false} />
                <XAxis dataKey="timestamp" stroke="#71717a" fontSize={12} tickLine={false} axisLine={false} />
                <YAxis stroke="#71717a" fontSize={12} tickLine={false} axisLine={false} />
                <Tooltip 
                  contentStyle={{ backgroundColor: '#111113', border: '1px solid #27272a', borderRadius: '8px' }}
                  itemStyle={{ color: '#e1e1e1' }}
                />
                <Line type="monotone" dataKey="node1_throughput" stroke="#3b82f6" strokeWidth={2} dot={false} name="Node 1" />
                <Line type="monotone" dataKey="node2_throughput" stroke="#a855f7" strokeWidth={2} dot={false} name="Node 2" />
                <Line type="monotone" dataKey="node3_throughput" stroke="#10b981" strokeWidth={2} dot={false} name="Node 3" />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Live Updates Stream */}
        <div className="glass p-6 flex flex-col h-[400px]">
          <h3 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Terminal size={20} className="text-green-400" />
            Live Replication Stream
          </h3>
          <div className="flex-1 overflow-y-auto space-y-3 pr-2 scrollbar-hide">
            {liveUpdates.length === 0 && (
              <div className="h-full flex flex-col items-center justify-center text-gray-500 text-center">
                <AlertCircle size={32} className="mb-2 opacity-20" />
                <p>No live updates yet.<br/>Modify 'test-watch' key to see stream.</p>
              </div>
            )}
            {liveUpdates.map((update) => (
              <div key={update.id} className="bg-secondary/50 border border-border p-3 rounded-lg animate-in fade-in slide-in-from-right-4">
                <div className="flex justify-between text-xs text-gray-500 mb-1">
                  <span>{new Date(update.timestamp / 1e6).toLocaleTimeString()}</span>
                  <span className="text-blue-400">v{Object.values(update.version).reduce((a,b) => a+b, 0)}</span>
                </div>
                <p className="font-mono text-sm break-all">{atob(update.data)}</p>
                {update.deleted && <span className="text-[10px] bg-red-500/20 text-red-400 px-1 rounded">TOMBSTONE</span>}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};

const StatCard = ({ icon, label, value }) => (
  <div className="glass p-6 flex items-center gap-4 hover:border-gray-600 transition-colors">
    <div className="p-3 bg-secondary rounded-lg">
      {icon}
    </div>
    <div>
      <p className="text-sm text-gray-400 font-medium">{label}</p>
      <p className="text-2xl font-bold">{value}</p>
    </div>
  </div>
);

export default Dashboard;
