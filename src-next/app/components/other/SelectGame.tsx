interface SelectGameProps {
    isOpen: boolean
    setIsOpen: (value: boolean) => void
}

export default function SelectGame({ isOpen, setIsOpen }: SelectGameProps) {
    return (
        isOpen && (
            <div className="modal modal-open text-base-content">
                <div className="modal-box">
                    <fieldset className="fieldset bg-base-200 border-base-300 rounded-box w-full border p-4">
                        <legend className="fieldset-legend text-md">游戏文件夹</legend>
                        <div className="join w-full">
                            <input type="text" className="input join-item " placeholder="选择游戏路径" />
                            <button className="btn join-item btn-soft">浏览</button>
                        </div>
                    </fieldset>
                    <fieldset className="fieldset bg-base-200 border-base-300 rounded-box w-full border p-4 mt-1">
                        <legend className="fieldset-legend text-md">游戏名称</legend>
                        <div className="join w-full">
                            <input type="text" className="input join-item " placeholder="起个名字" />
                        </div>
                    </fieldset>
                    <div className="pt-6" style={{ marginTop: '1.5rem' }}>
                        <button
                            className="btn  btn-error w-full"
                            onClick={() => setIsOpen(false)}
                        >
                            关闭
                        </button>
                    </div>
                    <div className="pt-6" style={{ marginTop: '0.5rem' }}>
                        <button
                            className="btn  btn-success w-full"
                            onClick={() => setIsOpen(false)}
                        >
                            添加
                        </button>
                    </div>
                </div>
            </div>
        )
    );
}