package tests

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubevirtv1 "kubevirt.io/kubevirt/pkg/api/v1"
)

var (
	gracePeriodSeconds int64 = 10
	rangeStart               = "02:00:00:00:00:00"
	rangeEnd                 = "02:FF:FF:FF:FF:FF"
)

var _ = Describe("Virtual Machines", func() {

	BeforeAll(func() {
		result := testClient.KubeClient.ExtensionsV1beta1().RESTClient().
			Post().
			RequestURI(fmt.Sprintf(postUrl, TestNamespace, "ovs-net-vlan100")).
			Body([]byte(fmt.Sprintf(ovsConfCRD, "ovs-net-vlan100", TestNamespace))).
			Do()
		Expect(result.Error()).NotTo(HaveOccurred())
	})

	Context("Check the client", func() {
		AfterEach(func() {
			vmList := &kubevirtv1.VirtualMachineList{}
			err := testClient.VirtClient.List(context.TODO(), &client.ListOptions{}, vmList)
			Expect(err).ToNot(HaveOccurred())

			for _, vmObject := range vmList.Items {
				err = testClient.VirtClient.Delete(context.TODO(), &vmObject)
				Expect(err).ToNot(HaveOccurred())
			}

			Eventually(func() int {
				vmList := &kubevirtv1.VirtualMachineList{}
				err := testClient.VirtClient.List(context.TODO(), &client.ListOptions{}, vmList)
				Expect(err).ToNot(HaveOccurred())

				return len(vmList.Items)

			}, 30*time.Second, 3*time.Second).Should(Equal(0), "failed to remove all vm objects")
		})

		Context("When the client wants to create a vm", func() {
			It("should create a vm object and automatically assign a static MAC address", func() {
				err := setRange(rangeStart, rangeEnd)
				Expect(err).ToNot(HaveOccurred())

				vm := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs", "")},
					[]kubevirtv1.Network{newNetwork("ovs")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm)
				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

				Expect(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

				_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())
			})
		})
		//2166
		Context("When the client tries to assign the same MAC address for two different vm. Within Range and out of range", func() {
			Context("When the MAC address is within range", func() {
				It("should reject a vm creation with an already allocated MAC address", func() {
					err := setRange(rangeStart, rangeEnd)
					Expect(err).ToNot(HaveOccurred())

					vm := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs", "")}, []kubevirtv1.Network{newNetwork("ovs")})

					Eventually(func() error {
						return testClient.VirtClient.Create(context.TODO(), vm)
					}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

					Expect(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

					_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
					Expect(err).ToNot(HaveOccurred())

					vmOverlap := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovsOverlap", "")}, []kubevirtv1.Network{newNetwork("ovsOverlap")})
					// Allocated the same MAC address that was registered to the first vm
					vmOverlap.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress = vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress

					Eventually(func() error {
						err = testClient.VirtClient.Create(context.TODO(), vmOverlap)
						if err != nil && strings.Contains(err.Error(), "failed to allocate requested mac address") {
							return err
						}
						return nil

					}, 40*time.Second, 5*time.Second).Should(HaveOccurred())
				})
			})
			//2167
			Context("When the MAC address is out of range", func() {
				It("should reject a vm creation with an already allocated MAC address", func() {
					err := setRange(rangeStart, rangeEnd)
					Expect(err).ToNot(HaveOccurred())

					vm := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs", "fe:ff:ff:ff:ff:ff")},
						[]kubevirtv1.Network{newNetwork("ovs")})
					Expect(len(vm.Spec.Template.Spec.Domain.Devices.Interfaces) > 0).To(BeTrue())
					Expect(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

					_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() error {
						return testClient.VirtClient.Create(context.TODO(), vm)

					}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

					vmOverlap := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovsOverlap", "fe:ff:ff:ff:ff:ff")},
						[]kubevirtv1.Network{newNetwork("ovsOverlap")})
					// Allocated the same mac address that was registered to the first vm
					vmOverlap.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress = vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress

					Eventually(func() error {
						err = testClient.VirtClient.Create(context.TODO(), vm)
						if err != nil && strings.Contains(err.Error(), "failed to allocate requested mac address") {
							return err
						}
						return nil

					}, 40*time.Second, 5*time.Second).Should(HaveOccurred())
				})
			})
		})
		//2199
		Context("when the client tries to assign the same MAC address for two different interfaces in a single VM.", func() {
			Context("When the MAC address is within range", func() {
				It("should reject a VM creation with two interfaces that share the same MAC address", func() {
					err := setRange(rangeStart, rangeEnd)
					Expect(err).ToNot(HaveOccurred())

					vm := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs1", "02:00:00:00:ff:ff"),
						newInterface("ovs2", "02:00:00:00:ff:ff")}, []kubevirtv1.Network{newNetwork("ovs1"), newNetwork("ovs2")})

					_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress).To(Not(BeEmpty()))

					_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress)
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() error {
						err = testClient.VirtClient.Create(context.TODO(), vm)
						if err != nil && strings.Contains(err.Error(), "failed to allocate requested mac address") {
							return err
						}
						return nil

					}, 40*time.Second, 5*time.Second).Should(HaveOccurred())
				})
			})
			//2200
			Context("When the MAC address is out of range", func() {
				It("should	reject a VM creation with two interfaces that share the same MAC address", func() {
					err := setRange(rangeStart, rangeEnd)
					Expect(err).ToNot(HaveOccurred())

					vm := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs1", "fe:ff:ff:ff:ff:ff"),
						newInterface("ovs2", "fe:ff:ff:ff:ff:ff")}, []kubevirtv1.Network{newNetwork("ovs1"), newNetwork("ovs2")})

					_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress).To(Not(BeEmpty()))

					_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress)
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() error {
						err = testClient.VirtClient.Create(context.TODO(), vm)
						if err != nil && strings.Contains(err.Error(), "failed to allocate requested mac address") {
							return err
						}
						return nil

					}, 40*time.Second, 5*time.Second).Should(HaveOccurred())
				})
			})
		})
		//2164
		Context("When two VM are deleted and we try to assign their MAC addresses for two newly created VM", func() {
			It("should not return an error because the MAC addresses of the old VMs should have been released", func() {

				err := setRange(rangeStart, rangeEnd)
				Expect(err).ToNot(HaveOccurred())

				//creating two Vm
				vm1 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs1", "")}, []kubevirtv1.Network{newNetwork("ovs1")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm1)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

				Expect(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

				_, err = net.ParseMAC(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				vm2 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs2", "")}, []kubevirtv1.Network{newNetwork("ovs2")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm2)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")
				Expect(vm2.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

				_, err = net.ParseMAC(vm2.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				vm1MacAddress := vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress
				vm2MacAddress := vm2.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress

				//deleting both and try to assign their MAC address to the new VM
				err = testClient.VirtClient.Delete(context.TODO(), vm1)
				Expect(err).ToNot(HaveOccurred())

				err = testClient.VirtClient.Delete(context.TODO(), vm2)
				Expect(err).ToNot(HaveOccurred())

				newVM1 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("newOvs1", vm1MacAddress)},
					[]kubevirtv1.Network{newNetwork("newOvs1")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), newVM1)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

				_, err = net.ParseMAC(newVM1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				newVM2 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("newOvs2", vm2MacAddress)},
					[]kubevirtv1.Network{newNetwork("newOvs2")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), newVM2)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

				_, err = net.ParseMAC(newVM2.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())
			})
		})
		//2162
		Context("When trying to create a VM after all MAC addresses in range have been occupied", func() {
			It("should return an error because no MAC address is available", func() {

				err := setRange("02:00:00:00:00:00", "02:00:00:00:00:01")
				Expect(err).ToNot(HaveOccurred())

				vm := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs1", ""),
					newInterface("ovs2", "")}, []kubevirtv1.Network{newNetwork("ovs1"), newNetwork("ovs2")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

				_, err = net.ParseMAC(vm.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				starvingVM := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("starvingOvs", "")},
					[]kubevirtv1.Network{newNetwork("starvingOvs")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), starvingVM)

				}, 40*time.Second, 5*time.Second).Should((HaveOccurred()), "failed to apply the new vm object")

			})
		})
		//2165
		Context("when trying to create a VM after a MAC address has just been released duo to a VM deletion", func() {
			It("should re-use the released MAC address for the creation of the new VM and not return an error", func() {
				err := setRange("02:00:00:00:00:00", "02:00:00:00:00:02")
				Expect(err).ToNot(HaveOccurred())

				vm1 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs1", ""),
					newInterface("ovs2", "")}, []kubevirtv1.Network{newNetwork("ovs1"), newNetwork("ovs2")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm1)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")

				Expect(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())
				Expect(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress).ToNot(BeEmpty())

				mac1, err := net.ParseMAC(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				mac2, err := net.ParseMAC(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				vm2 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs3", "")},
					[]kubevirtv1.Network{newNetwork("ovs3")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm2)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object error")
				Expect(vm2.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

				_, err = net.ParseMAC(vm2.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				err = testClient.VirtClient.Delete(context.TODO(), vm1)
				Expect(err).ToNot(HaveOccurred())

				newVM1 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("newOvs1", ""),
					newInterface("newOvs2", "")}, []kubevirtv1.Network{newNetwork("newOvs1"), newNetwork("newOvs2")})
				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), newVM1)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")
				Expect(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

				newMac1, err := net.ParseMAC(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())
				Expect(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress).ToNot(BeEmpty())

				newMac2, err := net.ParseMAC(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[1].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				Expect(newMac1.String()).To(Equal(mac1.String()))
				Expect(newMac2.String()).To(Equal(mac2.String()))
			})
		})
		//2179
		Context("When restarting kubeMacPool and trying to create a VM with the same manually configured MAC as an older VM", func() {
			It("should return an error because the MAC address is taken by the older VM", func() {
				err := setRange(rangeStart, rangeEnd)
				Expect(err).ToNot(HaveOccurred())

				vm1 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs1", "02:00:ff:ff:ff:ff")}, []kubevirtv1.Network{newNetwork("ovs1")})

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm1)

				}, 40*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object")
				Expect(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress).ToNot(BeEmpty())

				_, err = net.ParseMAC(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[0].MacAddress)
				Expect(err).ToNot(HaveOccurred())

				//restart kubeMacPool
				err = setRange(rangeStart, rangeEnd)
				Expect(err).ToNot(HaveOccurred())

				vm2 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{newInterface("ovs2", "02:00:ff:ff:ff:ff")},
					[]kubevirtv1.Network{newNetwork("ovs2")})

				Eventually(func() error {
					err = testClient.VirtClient.Create(context.TODO(), vm2)
					if err != nil && strings.Contains(err.Error(), "failed to allocate requested mac address") {
						return err
					}
					return nil

				}, 40*time.Second, 5*time.Second).Should(HaveOccurred())

			})
		})
		//2243
		Context("When we re-apply a VM yaml", func() {
			It("should assign to the VM the same MAC addresses as before the re-apply, and not return an error", func() {
				err := setRange(rangeStart, rangeEnd)
				Expect(err).ToNot(HaveOccurred())

				intrefaces := make([]kubevirtv1.Interface, 5)
				networks := make([]kubevirtv1.Network, 5)
				for i := 0; i < 5; i++ {
					intrefaces[i] = newInterface("ovs"+strconv.Itoa(i), "")
					networks[i] = newNetwork("ovs" + strconv.Itoa(i))
				}

				vm1 := CreateVmObject(TestNamespace, false, []kubevirtv1.Interface{intrefaces[0], intrefaces[1],
					intrefaces[2], intrefaces[3], intrefaces[4]},
					[]kubevirtv1.Network{networks[0], networks[1], networks[2], networks[3], networks[4]})
				updateObject := vm1.DeepCopy()

				Eventually(func() error {
					return testClient.VirtClient.Create(context.TODO(), vm1)

				}, 50*time.Second, 5*time.Second).Should(Not(HaveOccurred()), "failed to apply the new vm object error")

				updateObject.ObjectMeta = *vm1.ObjectMeta.DeepCopy()

				for index := range vm1.Spec.Template.Spec.Domain.Devices.Interfaces {
					_, err = net.ParseMAC(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[index].MacAddress)
					Expect(err).ToNot(HaveOccurred())
				}

				err = testClient.VirtClient.Update(context.TODO(), updateObject)
				Expect(err).ToNot(HaveOccurred())

				for index := range vm1.Spec.Template.Spec.Domain.Devices.Interfaces {
					Expect(vm1.Spec.Template.Spec.Domain.Devices.Interfaces[index].MacAddress).To(Equal(updateObject.Spec.Template.Spec.Domain.Devices.Interfaces[index].MacAddress))
				}
			})
		})
	})
})

func setRange(rangeStart, rangeEnd string) error {
	configMap, err := testClient.KubeClient.CoreV1().ConfigMaps("kubemacpool-system").Get("kubemacpool-mac-range-config", metav1.GetOptions{})
	if err != nil {
		return err
	}

	configMap.Data["RANGE_START"] = rangeStart
	configMap.Data["RANGE_END"] = rangeEnd

	_, err = testClient.KubeClient.CoreV1().ConfigMaps("kubemacpool-system").Update(configMap)
	if err != nil {
		return err
	}

	podsList, err := testClient.KubeClient.CoreV1().Pods("kubemacpool-system").List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	err = testClient.KubeClient.CoreV1().Pods("kubemacpool-system").Delete(podsList.Items[0].Name, &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriodSeconds})
	return err
}

func newInterface(name, macAddress string) kubevirtv1.Interface {
	return kubevirtv1.Interface{
		Name: name,
		InterfaceBindingMethod: kubevirtv1.InterfaceBindingMethod{
			Bridge: &kubevirtv1.InterfaceBridge{},
		},
		MacAddress: macAddress,
	}
}

func newNetwork(name string) kubevirtv1.Network {
	return kubevirtv1.Network{
		Name: name,
		NetworkSource: kubevirtv1.NetworkSource{
			Multus: &kubevirtv1.MultusNetwork{
				NetworkName: name,
			},
		},
	}
}
