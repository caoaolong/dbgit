import { useState } from 'react'
import { Modal, Tabs } from 'antd'
import { InstallPanel } from '../pages/InstallModal'
import { NodeFormPanel } from './NodeModal'

type Props = {
  open: boolean
  onClose: () => void
  onChanged: () => void
}

export function ProxyAddModal({ open, onClose, onChanged }: Props) {
  const [activeKey, setActiveKey] = useState('local')

  return (
    <Modal
      title="添加代理"
      open={open}
      onCancel={onClose}
      footer={null}
      width={560}
      destroyOnHidden
    >
      <Tabs
        activeKey={activeKey}
        onChange={setActiveKey}
        items={[
          {
            key: 'local',
            label: '安装本地代理',
            children: <InstallPanel onInstalled={onChanged} />,
          },
          {
            key: 'remote',
            label: '添加远程代理',
            children: <NodeFormPanel initial={null} onSaved={onChanged} />,
          },
        ]}
      />
    </Modal>
  )
}
