import { FaJava } from "react-icons/fa";
import { LuFolderSearch } from "react-icons/lu";
import { BiImport } from "react-icons/bi";
export default function NormalContext() {
    return (
        <div>
            <div className="card w-full bg-base-100 card-xs shadow-sm">
                <div className="card-body text-base-content">
                    <div className="flex items-center gap-2 mb-2">
                        <FaJava className="text-2xl text-base-content" />
                        <h2 className="card-title m-0">Java路径</h2>
                    </div>

                    <div className="card-actions flex flex-col gap-3">
                        <select defaultValue="Xsmall" className="select select-sm w-full border border-gray-300 rounded-md">
                            <option disabled={false}>自动选择合适的Java</option>
                            <option>JDK 12.5 C:/java/bin/javae.exe</option>
                        </select>

                        <div className="">

                            <button className="btn btn-soft  btn-xs ">
                                <LuFolderSearch className="text-base text-base-content" />
                                <span className="text-xs">浏览</span>
                            </button>
                            <button className="btn btn-soft  btn-xs ml-3 ">
                                <BiImport className="text-base text-base-content" />
                                <span className="text-xs">手动导入</span>
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}